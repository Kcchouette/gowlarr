package cli

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Kcchouette/cardigann-go/definition"
)

func japanfanLikeDef() definition.IndexerDefinition {
	return definition.IndexerDefinition{
		Settings: []definition.Setting{
			{Name: "username", Type: "text", Label: "Username"},
			{Name: "password", Type: "password", Label: "Password"},
			{Name: "info_sitelink", Type: "info", Label: "Site", Default: "https://japanfan.org/phpBB3/"},
		},
	}
}

func abnormalLikeDef() definition.IndexerDefinition {
	return definition.IndexerDefinition{
		Settings: []definition.Setting{
			{Name: "apikey", Type: "text", Label: "APIKey"},
			{Name: "info_key", Type: "info", Label: "About your API key", Default: "Find your API key on your Parameters page."},
			{Name: "apiurl", Type: "text", Label: "API URL", Default: "api.abn.lol"},
			{Name: "freeleech", Type: "checkbox", Label: "Freeleech only", Default: "false"},
			{Name: "multilanguage", Type: "select", Label: "Language", Default: "FRENCH"},
			{Name: "sort", Type: "select", Label: "Sort", Default: "Created"},
		},
	}
}

func TestRequiredSettings(t *testing.T) {
	tests := []struct {
		name     string
		def      definition.IndexerDefinition
		provided map[string]string
		want     []string
	}{
		{
			"japanfan nothing provided",
			japanfanLikeDef(),
			nil,
			[]string{"username", "password"},
		},
		{
			"japanfan username provided",
			japanfanLikeDef(),
			map[string]string{"username": "alice"},
			[]string{"password"},
		},
		{
			"japanfan all provided",
			japanfanLikeDef(),
			map[string]string{"username": "alice", "password": "secret"},
			nil,
		},
		{
			"japanfan empty value counts as provided",
			japanfanLikeDef(),
			map[string]string{"password": ""},
			[]string{"username"},
		},
		{
			"abnormal only apikey required",
			abnormalLikeDef(),
			nil,
			[]string{"apikey"},
		},
		{
			"abnormal apikey provided",
			abnormalLikeDef(),
			map[string]string{"apikey": "k"},
			nil,
		},
		{
			"select without default is not required",
			definition.IndexerDefinition{Settings: []definition.Setting{
				{Name: "mode", Type: "select", Label: "Mode"},
			}},
			nil,
			nil,
		},
		{
			"unknown or empty type is not required",
			definition.IndexerDefinition{Settings: []definition.Setting{
				{Name: "weird", Type: "custom"},
				{Name: "empty", Type: ""},
			}},
			nil,
			nil,
		},
		{
			"empty settings list",
			definition.IndexerDefinition{},
			nil,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requiredSettings(tt.def, tt.provided)
			names := make([]string, 0, len(got))
			for _, s := range got {
				names = append(names, s.Name)
			}
			if len(names) == 0 {
				names = nil
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Errorf("requiredSettings = %v, want %v", names, tt.want)
			}
		})
	}
}

// TestStdinPrompterPlain exercises the real stdinPrompter plain path through
// a pipe: the label is printed and the trimmed line is returned.
// The masked path (term.ReadPassword) is not testable without a PTY — the
// label print shares the same hoisted code, covered here.
func TestStdinPrompterPlain(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
		_ = w.Close()
	})

	go func() {
		_, _ = w.Write([]byte("alice\n"))
		_ = w.Close()
	}()

	var out bytes.Buffer
	got, err := stdinPrompter{out: &out}.Ask("Username", false, "help text")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got != "alice" {
		t.Errorf("Ask = %q, want alice", got)
	}
	if !strings.Contains(out.String(), "help text") || !strings.Contains(out.String(), "Username:") {
		t.Errorf("output missing label/help: %q", out.String())
	}
}

// TestStdinPrompterPlainEOF covers the EOF-without-line error path.
func TestStdinPrompterPlainEOF(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
		_ = w.Close()
	})
	_ = w.Close() // immediate EOF

	_, err = stdinPrompter{out: io.Discard}.Ask("Username", false, "")
	if err == nil || !strings.Contains(err.Error(), "input closed (EOF)") {
		t.Errorf("expected EOF error, got %v", err)
	}
}
