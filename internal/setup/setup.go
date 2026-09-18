// Package setup runs the post-create steps from the project config.
package setup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mklinovsky/worq/internal/config"
)

// Event reports progress while steps run.
type Event struct {
	Index int
	Total int
	Label string
	Line  string // output line, when non-empty
	Done  bool
	Err   error
}

// Reporter receives events. Nil is allowed.
type Reporter func(Event)

// Result records the outcome of one step.
type Result struct {
	Label string
	Err   error
}

// Run executes every step against a freshly created worktree. src is the main
// worktree (the source for copies and links), dst the new one.
func Run(steps []config.Step, src, dst string, report Reporter) []Result {
	results := make([]Result, 0, len(steps))
	for i, s := range steps {
		label := s.Label()
		// A step with nothing to do is a config mistake — usually a key that no
		// longer exists. Say so instead of reporting a silent success.
		if len(s.Copy) == 0 && s.Run == "" {
			err := errors.New("step has no copy or run — check " + label + " in your config")
			emit(report, Event{Index: i, Total: len(steps), Label: label, Done: true, Err: err})
			results = append(results, Result{Label: label, Err: fmt.Errorf("%w (optional)", err)})
			continue
		}
		emit(report, Event{Index: i, Total: len(steps), Label: label})

		err := runStep(s, src, dst, func(line string) {
			emit(report, Event{Index: i, Total: len(steps), Label: label, Line: line})
		})
		if err != nil && s.Optional {
			err = fmt.Errorf("%w (optional)", err)
		}
		emit(report, Event{Index: i, Total: len(steps), Label: label, Done: true, Err: err})
		results = append(results, Result{Label: label, Err: err})
		if err != nil && !s.Optional {
			break
		}
	}
	return results
}

// Failed returns the first non-optional failure, if any.
func Failed(results []Result) error {
	for _, r := range results {
		if r.Err != nil && !strings.HasSuffix(r.Err.Error(), "(optional)") {
			return r.Err
		}
	}
	return nil
}

func emit(r Reporter, e Event) {
	if r != nil {
		r(e)
	}
}

func runStep(s config.Step, src, dst string, out func(string)) error {
	switch {
	case len(s.Copy) > 0:
		return copyPaths(s.Copy, src, dst, out)
	case s.Run != "":
		dir := dst
		if s.Dir != "" {
			dir = filepath.Join(dst, s.Dir)
		}
		return shell(s.Run, dir, out)
	}
	return nil
}

func copyPaths(paths []string, src, dst string, out func(string)) error {
	for _, rel := range paths {
		from := filepath.Join(src, rel)
		to := filepath.Join(dst, rel)
		info, err := os.Lstat(from)
		if errors.Is(err, os.ErrNotExist) {
			out("skipped " + rel + " (not in " + filepath.Base(src) + ")")
			continue
		}
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if info.IsDir() {
			if err := copyTree(from, to); err != nil {
				return err
			}
		} else if err := copyFile(from, to, info.Mode()); err != nil {
			return err
		}
		out("copied " + rel)
	}
	return nil
}

func copyFile(from, to string, mode os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := to + ".worq-tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, in); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, to)
}

func copyTree(from, to string) error {
	return filepath.Walk(from, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			dest, err := os.Readlink(p)
			if err != nil {
				return err
			}
			os.Remove(target)
			return os.Symlink(dest, target)
		default:
			return copyFile(p, target, info.Mode())
		}
	})
}

// shell runs a command with the user's shell so config can use pipes, && and
// aliases-free one-liners, streaming combined output line by line.
func shell(command, dir string, out func(string)) error {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	cmd := exec.Command(sh, "-c", command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "WORQ=1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			out(sc.Text())
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	wg.Wait()
	return cmd.Wait()
}
