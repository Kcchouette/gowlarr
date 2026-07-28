package cli

import "testing"

func TestHumanSize(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{500, "500B"},
		{1024, "1.0KiB"},
		{1048576, "1.0MiB"},
		{1073741824, "1.0GiB"},
	}
	for _, tc := range cases {
		got := humanSize(tc.bytes)
		if got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}
