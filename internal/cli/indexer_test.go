package cli

import "testing"

func TestIsNewznabAlias(t *testing.T) {
	positives := []string{"newznab", "nzb", "usenet", " Newznab ", "NZB", "USENET"}
	for _, q := range positives {
		if !isNewznabAlias(q) {
			t.Errorf("expected %q to be a newznab alias", q)
		}
	}
	negatives := []string{"usenet-crawler", "1337x", "newznab-plus", "hydra", ""}
	for _, q := range negatives {
		if isNewznabAlias(q) {
			t.Errorf("expected %q NOT to be a newznab alias", q)
		}
	}
}

func TestNewznabDefaultID(t *testing.T) {
	cases := []struct{ baseURL, want string }{
		{"https://api.nzbgeek.info/", "api.nzbgeek.info"},
		{"http://nzb.su:8080/api", "nzb.su"},
		{"https://api.example.invalid", "api.example.invalid"},
		{"", "newznab"},
		{"not a url", "newznab"},
	}
	for _, c := range cases {
		if got := newznabDefaultID(c.baseURL); got != c.want {
			t.Errorf("newznabDefaultID(%q) = %q, want %q", c.baseURL, got, c.want)
		}
	}
}
