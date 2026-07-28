package newznab

import (
	"testing"
)

func TestCategoryNameToID(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"Movies", 2000},
		{"TV", 3000},
		{"TV/HD", 3040},
		{"Audio/MP3", 4010},
		{"Books/EBook", 5010},
		{"PC/Games", 6010},
		{"Console", 7000},
		{"XXX", 8000},
		{"Unknown", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CategoryNameToID(tt.name)
			if got != tt.want {
				t.Errorf("CategoryNameToID(%q) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestCategoryIDToName(t *testing.T) {
	tests := []struct {
		id   int
		want string
	}{
		{2000, "Movies"},
		{3000, "TV"},
		{3040, "TV/HD"},
		{4010, "Audio/MP3"},
		{5010, "Books/EBook"},
		{0, ""},
		{9999, ""},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := CategoryIDToName(tt.id)
			if got != tt.want {
				t.Errorf("CategoryIDToName(%d) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}
