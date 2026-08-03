package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Kcchouette/cardigann-go/definition"
)

func credsLikeDef() definition.IndexerDefinition {
	return definition.IndexerDefinition{
		Settings: []definition.Setting{
			{Name: "username", Type: "text", Label: "Username"},
			{Name: "password", Type: "password", Label: "Password"},
			{Name: "info_sitelink", Type: "info", Label: "Site", Default: "https://example.invalid/forum/"},
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
			"creds nothing provided",
			credsLikeDef(),
			nil,
			[]string{"username", "password"},
		},
		{
			"creds username provided",
			credsLikeDef(),
			map[string]string{"username": "alice"},
			[]string{"password"},
		},
		{
			"creds all provided",
			credsLikeDef(),
			map[string]string{"username": "alice", "password": "secret"},
			nil,
		},
		{
			"creds empty value counts as provided",
			credsLikeDef(),
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

type fakePromptCall struct {
	label  string
	masked bool
	help   string
}

type fakePrompter struct {
	answers []string
	err     error
	calls   []fakePromptCall
}

func (f *fakePrompter) Ask(label string, masked bool, help string) (string, error) {
	f.calls = append(f.calls, fakePromptCall{label: label, masked: masked, help: help})
	if f.err != nil {
		return "", f.err
	}
	if len(f.answers) == 0 {
		return "", nil
	}
	v := f.answers[0]
	f.answers = f.answers[1:]
	return v, nil
}

func TestRunPrompts(t *testing.T) {
	t.Run("no required settings", func(t *testing.T) {
		fake := &fakePrompter{}
		var buf bytes.Buffer
		got, err := runPrompts(credsLikeDef(), map[string]string{"username": "a", "password": "b"}, fake, &buf)
		if err != nil {
			t.Fatalf("runPrompts: %v", err)
		}
		if len(fake.calls) != 0 {
			t.Errorf("expected no prompts, got %d", len(fake.calls))
		}
		if got["username"] != "a" || got["password"] != "b" {
			t.Errorf("map changed unexpectedly: %v", got)
		}
	})

	t.Run("creds prompts in order with masked password and info once", func(t *testing.T) {
		fake := &fakePrompter{answers: []string{"alice", "s3cret"}}
		var buf bytes.Buffer
		got, err := runPrompts(credsLikeDef(), nil, fake, &buf)
		if err != nil {
			t.Fatalf("runPrompts: %v", err)
		}
		if len(fake.calls) != 2 {
			t.Fatalf("expected 2 prompts, got %d", len(fake.calls))
		}
		if fake.calls[0] != (fakePromptCall{label: "Username", masked: false}) ||
			fake.calls[1] != (fakePromptCall{label: "Password", masked: true}) {
			t.Errorf("calls = %+v", fake.calls)
		}
		if got["username"] != "alice" || got["password"] != "s3cret" {
			t.Errorf("merged = %v", got)
		}
		if n := strings.Count(buf.String(), "https://example.invalid/forum/"); n != 1 {
			t.Errorf("info help printed %d times, want 1", n)
		}
	})

	t.Run("abnormal with apikey provided prompts nothing", func(t *testing.T) {
		fake := &fakePrompter{}
		var buf bytes.Buffer
		got, err := runPrompts(abnormalLikeDef(), map[string]string{"apikey": "k"}, fake, &buf)
		if err != nil {
			t.Fatalf("runPrompts: %v", err)
		}
		if len(fake.calls) != 0 {
			t.Errorf("expected no prompts, got %d", len(fake.calls))
		}
		if got["apikey"] != "k" {
			t.Errorf("apikey = %q, want k", got["apikey"])
		}
	})

	t.Run("empty answer accepted", func(t *testing.T) {
		fake := &fakePrompter{answers: []string{""}}
		var buf bytes.Buffer
		got, err := runPrompts(credsLikeDef(), map[string]string{"username": "a"}, fake, &buf)
		if err != nil {
			t.Fatalf("runPrompts: %v", err)
		}
		if got["password"] != "" {
			t.Errorf("password = %q, want empty", got["password"])
		}
	})

	t.Run("prompter error propagates with setting name", func(t *testing.T) {
		fake := &fakePrompter{err: errors.New("boom")}
		var buf bytes.Buffer
		_, err := runPrompts(credsLikeDef(), nil, fake, &buf)
		if err == nil || !strings.Contains(err.Error(), "username") {
			t.Errorf("expected error mentioning username, got %v", err)
		}
	})

	t.Run("two info settings printed once each", func(t *testing.T) {
		def := definition.IndexerDefinition{Settings: []definition.Setting{
			{Name: "info1", Type: "info", Default: "HELP ONE"},
			{Name: "user", Type: "text", Label: "User"},
			{Name: "info2", Type: "info", Default: "HELP TWO"},
			{Name: "pass", Type: "password", Label: "Pass"},
		}}
		fake := &fakePrompter{answers: []string{"u", "p"}}
		var buf bytes.Buffer
		if _, err := runPrompts(def, nil, fake, &buf); err != nil {
			t.Fatalf("runPrompts: %v", err)
		}
		out := buf.String()
		if strings.Count(out, "HELP ONE") != 1 || strings.Count(out, "HELP TWO") != 1 {
			t.Errorf("info help printed wrong number of times: %q", out)
		}
	})
}

func TestPrintSettingsSummary(t *testing.T) {
	t.Run("password masked, definition order", func(t *testing.T) {
		var buf bytes.Buffer
		printSettingsSummary(credsLikeDef(), map[string]string{"password": "s3cret", "username": "alice"}, &buf)
		out := buf.String()
		if strings.Contains(out, "s3cret") {
			t.Errorf("password leaked: %q", out)
		}
		if !strings.Contains(out, "*****") {
			t.Errorf("expected masked password: %q", out)
		}
		// definition order: Username before Password
		ui, pi := strings.Index(out, "Username"), strings.Index(out, "Password")
		if ui < 0 || pi < 0 || ui > pi {
			t.Errorf("definition order not preserved: %q", out)
		}
	})

	t.Run("label fallback to name", func(t *testing.T) {
		def := definition.IndexerDefinition{Settings: []definition.Setting{
			{Name: "apikey", Type: "text"},
		}}
		var buf bytes.Buffer
		printSettingsSummary(def, map[string]string{"apikey": "k"}, &buf)
		if !strings.Contains(buf.String(), "apikey: k") {
			t.Errorf("expected name fallback: %q", buf.String())
		}
	})

	t.Run("unknown key listed after, sorted", func(t *testing.T) {
		var buf bytes.Buffer
		printSettingsSummary(credsLikeDef(), map[string]string{"username": "a", "zzz": "v", "mmm": "v"}, &buf)
		out := buf.String()
		if strings.Index(out, "mmm: v") > strings.Index(out, "zzz: v") {
			t.Errorf("extra keys not sorted: %q", out)
		}
	})

	t.Run("empty map silent", func(t *testing.T) {
		var buf bytes.Buffer
		printSettingsSummary(credsLikeDef(), nil, &buf)
		if buf.Len() != 0 {
			t.Errorf("expected no output, got %q", buf.String())
		}
	})
}

const needsCredsYAML = `id: needs-creds
name: Needs Creds
description: interactive add test
language: en-US
type: private
links:
  - https://needs-creds.example.com
settings:
  - name: username
    type: text
    label: Username
  - name: password
    type: password
    label: Password
  - name: info_key
    type: info
    label: About your API key
    default: "Find your API key on the Parameters page."
search:
  paths:
    - path: /search
      response:
        type: html
  rows:
    selector: tr
  fields:
    title:
      selector: a
`

// TestIndexerAddCmdNonTTYMissingSettings covers the non-interactive guard:
// with stdin not a terminal and required settings missing, add fails with a
// --setting hint (plus the definition's info help) and persists nothing.
func TestIndexerAddCmdNonTTYMissingSettings(t *testing.T) {
	isolateCLIUserDirs(t)
	writeCLIConfig(t)

	st, _, err := openStore()
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	if err := st.SaveDefinition("needs-creds", "v11", "sha", needsCredsYAML); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	st.Close()

	// Force stdin to a pipe: the real term.IsTerminal path returns false.
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

	var errBuf bytes.Buffer
	cmd := newIndexerAddCmd()
	cmd.SetArgs([]string{"needs-creds"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(&errBuf)
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected missing-settings error, got nil")
	}
	for _, want := range []string{"requires settings", "username", "password", "--setting"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
	// The definition's info help is printed on stderr (the error message
	// carries the keys and the hint).
	if !strings.Contains(errBuf.String(), "Parameters page") {
		t.Errorf("info help missing on stderr: %q", errBuf.String())
	}

	// Nothing must have been persisted.
	st2, _, err := openStore()
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	defer st2.Close()
	cfgs, err := st2.ListIndexerConfigs(false, nil)
	if err != nil {
		t.Fatalf("ListIndexerConfigs: %v", err)
	}
	if len(cfgs) != 0 {
		t.Errorf("indexer persisted despite the error: %+v", cfgs)
	}
}

// TestIndexerAddCmdInteractiveFillsSettings covers the interactive path: the
// fake prompter answers the required settings, the summary prints on stderr,
// and the merged settings are persisted.
func TestIndexerAddCmdInteractiveFillsSettings(t *testing.T) {
	isolateCLIUserDirs(t)
	writeCLIConfig(t)

	st, _, err := openStore()
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	if err := st.SaveDefinition("needs-creds", "v11", "sha", needsCredsYAML); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}
	st.Close()

	oldTTY, oldNew := stdinIsTerminal, newStdinPrompter
	stdinIsTerminal = func() bool { return true }
	newStdinPrompter = func(w io.Writer) prompter {
		return &fakePrompter{answers: []string{"alice", "s3cret"}}
	}
	t.Cleanup(func() { stdinIsTerminal, newStdinPrompter = oldTTY, oldNew })

	var errBuf bytes.Buffer
	cmd := newIndexerAddCmd()
	cmd.SetArgs([]string{"needs-creds"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(&errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The summary goes to stderr with the password masked.
	if !strings.Contains(errBuf.String(), "Settings to save:") {
		t.Errorf("summary missing on stderr: %q", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "s3cret") {
		t.Errorf("password leaked in summary: %q", errBuf.String())
	}

	st2, _, err := openStore()
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	defer st2.Close()
	cfgs, err := st2.ListIndexerConfigs(false, nil)
	if err != nil {
		t.Fatalf("ListIndexerConfigs: %v", err)
	}
	if len(cfgs) != 1 {
		t.Fatalf("expected 1 persisted indexer, got %d", len(cfgs))
	}
	if cfgs[0].Settings["username"] != "alice" || cfgs[0].Settings["password"] != "s3cret" {
		t.Errorf("persisted settings = %v, want username=alice password=s3cret", cfgs[0].Settings)
	}
}
