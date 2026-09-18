package naming

import "testing"

func TestBuild(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		jira       string
		prefix     string
		wantBranch string
		wantDir    string
	}{
		{"jira key and summary", []string{"PROJ-123", "fix", "login"}, "", "", "proj-123-fix-login", "proj-123-fix-login"},
		{"lowercase key normalised", []string{"proj-123"}, "", "", "proj-123", "proj-123"},
		{"bare number uses project key", []string{"123", "fix login"}, "PROJ", "", "proj-123-fix-login", "proj-123-fix-login"},
		{"bare number without key falls back to slug", []string{"123", "fix"}, "", "", "123-fix", "123-fix"},
		{"plain description with prefix", []string{"fix login redirect"}, "", "feat/", "feat/fix-login-redirect", "feat-fix-login-redirect"},
		{"explicit branch kept", []string{"feat/mobile/login"}, "", "feat/", "feat/mobile/login", "feat-mobile-login"},
		{"punctuation collapses", []string{"PROJ-9", "Fix:  the *thing*!"}, "", "", "proj-9-fix-the-thing", "proj-9-fix-the-thing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Build(c.args, c.jira, c.prefix)
			if got.Branch != c.wantBranch || got.Dir != c.wantDir {
				t.Fatalf("Build(%q) = %+v, want branch=%q dir=%q", c.args, got, c.wantBranch, c.wantDir)
			}
		})
	}
}
