package update

import "testing"

func TestParseRepoAndRef(t *testing.T) {
	const defaultRepo = "DockSTARTer-Templates"

	cases := []struct {
		name     string
		spec     string
		wantSlug string
		wantRef  string
	}{
		{"bare ref", "main", "", "main"},
		{"empty spec", "", "", ""},
		{"owner and repo", "someuser/DockSTARTer-Templates@my-branch", "someuser/DockSTARTer-Templates", "my-branch"},
		{"owner only shorthand", "someuser@my-branch", "someuser/DockSTARTer-Templates", "my-branch"},
		{"owner only, empty ref", "someuser@", "someuser/DockSTARTer-Templates", ""},
		{"owner with differently named repo", "someuser/forked-templates@main", "someuser/forked-templates", "main"},
		{"reflog syntax untouched", "HEAD@{2}", "", "HEAD@{2}"},
		{"reflog syntax with branch prefix untouched", "main@{upstream}", "", "main@{upstream}"},
		{"leading @ with no owner", "@main", "", "main"},
		{"bare @ alone", "@", "", ""},
		{"tag ref", "v1.2.3.4", "", "v1.2.3.4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSlug, gotRef := ParseRepoAndRef(tc.spec, defaultRepo)
			if gotSlug != tc.wantSlug || gotRef != tc.wantRef {
				t.Errorf("ParseRepoAndRef(%q, %q) = (%q, %q), want (%q, %q)",
					tc.spec, defaultRepo, gotSlug, gotRef, tc.wantSlug, tc.wantRef)
			}
		})
	}
}

func TestRepoDisplayPrefix(t *testing.T) {
	const defaultRepoName = "DockSTARTer-Templates"

	cases := []struct {
		name     string
		repoSlug string
		want     string
	}{
		{"empty slug", "", ""},
		{"same-named fork", "someuser/DockSTARTer-Templates", "someuser@"},
		{"differently named repo", "someuser/forked-templates", "someuser/forked-templates@"},
		{"malformed slug (no slash)", "someuser", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := repoDisplayPrefix(tc.repoSlug, defaultRepoName)
			if got != tc.want {
				t.Errorf("repoDisplayPrefix(%q, %q) = %q, want %q", tc.repoSlug, defaultRepoName, got, tc.want)
			}
		})
	}
}

func TestRepoSlugFromURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/GhostWriters/DockSTARTer2", "GhostWriters/DockSTARTer2"},
		{"https://github.com/GhostWriters/DockSTARTer2.git", "GhostWriters/DockSTARTer2"},
		{"https://github.com/someuser/forked-templates.git", "someuser/forked-templates"},
	}

	for _, tc := range cases {
		got := repoSlugFromURL(tc.url)
		if got != tc.want {
			t.Errorf("repoSlugFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}
