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
		{"jira key and summary", []string{"EFRON-123", "fix", "login"}, "", "", "efron-123-fix-login", "efron-123-fix-login"},
		{"lowercase key normalised", []string{"efron-123"}, "", "", "efron-123", "efron-123"},
		{"bare number uses project key", []string{"123", "fix login"}, "EFRON", "", "efron-123-fix-login", "efron-123-fix-login"},
		{"bare number without key falls back to slug", []string{"123", "fix"}, "", "", "123-fix", "123-fix"},
		{"plain description with prefix", []string{"fix login redirect"}, "", "feat/", "feat/fix-login-redirect", "feat-fix-login-redirect"},
		{"explicit branch kept", []string{"feat/mobile/login"}, "", "feat/", "feat/mobile/login", "feat-mobile-login"},
		{"punctuation collapses", []string{"EFRON-9", "Fix:  the *thing*!"}, "", "", "efron-9-fix-the-thing", "efron-9-fix-the-thing"},
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
