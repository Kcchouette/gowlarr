package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/Kcchouette/cardigann-go/definition"
	"github.com/charmbracelet/x/term"
)

// requiredSettings returns the settings that must be supplied before the
// definition is usable: text/password settings without a Default, in
// definition order, skipping keys already present in provided (presence
// counts even with an empty value — consistent with --setting key=).
// Settings with a Default are injected by the engine at search time
// (configOnce), so they are never prompted for.
func requiredSettings(def definition.IndexerDefinition, provided map[string]string) []definition.Setting {
	required := make([]definition.Setting, 0, len(def.Settings))
	for _, s := range def.Settings {
		if (s.Type == "text" || s.Type == "password") && s.Default == "" {
			if _, ok := provided[s.Name]; !ok {
				required = append(required, s)
			}
		}
	}
	return required
}

// stdinIsTerminal reports whether stdin is attached to a terminal (and we
// are not in CI). It is a package variable so tests can force either path.
// A CI/pty/ssh -t session reports a TTY while still being scripted: honor
// CI to avoid blocking forever on a prompt nobody will answer.
var stdinIsTerminal = func() bool {
	if os.Getenv("CI") != "" {
		return false
	}
	return term.IsTerminal(os.Stdin.Fd())
}

// newStdinPrompter constructs the real prompter; overridable in tests to
// inject a fake without a PTY.
var newStdinPrompter = func(w io.Writer) prompter {
	return stdinPrompter{out: w}
}

// prompter is the interactive input seam; tests inject a fake.
type prompter interface {
	// Ask reads one value for a setting. masked disables echo (password).
	Ask(label string, masked bool, help string) (string, error)
}

// stdinPrompter prompts on the real terminal.
type stdinPrompter struct {
	out io.Writer
}

func (p stdinPrompter) Ask(label string, masked bool, help string) (string, error) {
	if help != "" {
		fmt.Fprintln(p.out, help)
	}
	// The label is printed for BOTH paths: without it a masked prompt would
	// give the user no cue before typing.
	fmt.Fprintf(p.out, "%s: ", label)
	if masked {
		// Intercept SIGINT so the process does not die mid-read: x/term's
		// ReadPassword restores the terminal echo in a defer, which only runs
		// if the read returns (EINTR) instead of the default handler killing
		// the process with echo still disabled.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		v, err := term.ReadPassword(os.Stdin.Fd())
		signal.Stop(ch)
		fmt.Fprintln(p.out) // newline: ReadPassword suppresses echo AND the Enter
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", label, err)
		}
		return string(v), nil
	}
	// bufio.NewReader per call is safe on a terminal (canonical mode returns
	// one line per read); the prompter is TTY-gated by design.
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	if errors.Is(err, io.EOF) && line == "" {
		return "", fmt.Errorf("reading %s: input closed (EOF)", label)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// runPrompts interactively fills the missing required settings of def,
// printing each info setting's Default text once as help beforehand. The
// result merges prompted values into a copy of provided.
func runPrompts(def definition.IndexerDefinition, provided map[string]string, p prompter, w io.Writer) (map[string]string, error) {
	required := requiredSettings(def, provided)
	if len(required) == 0 {
		return provided, nil
	}
	for _, s := range def.Settings {
		if s.Type == "info" && s.Default != "" {
			fmt.Fprintln(w, s.Default)
		}
	}
	out := make(map[string]string, len(provided)+len(required))
	for k, v := range provided {
		out[k] = v
	}
	for _, s := range required {
		label := s.Label
		if label == "" {
			label = s.Name
		}
		v, err := p.Ask(label, s.Type == "password", "")
		if err != nil {
			return nil, fmt.Errorf("prompting %q: %w", s.Name, err)
		}
		out[s.Name] = v
	}
	return out, nil
}

// printSettingsSummary prints the settings about to be persisted, masking
// password values. Keys not declared in the definition (likely --setting
// typos) are listed after the known ones, sorted.
func printSettingsSummary(def definition.IndexerDefinition, settings map[string]string, w io.Writer) {
	if len(settings) == 0 {
		return
	}
	fmt.Fprintln(w, "Settings to save:")
	known := make(map[string]struct{}, len(def.Settings))
	for _, s := range def.Settings {
		known[s.Name] = struct{}{}
		if v, ok := settings[s.Name]; ok {
			fmt.Fprintf(w, "  %s: %s\n", displaySettingLabel(s), displaySettingValue(s, v))
		}
	}
	var extra []string
	for k := range settings {
		if _, ok := known[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		fmt.Fprintf(w, "  %s: %s\n", k, settings[k])
	}
}

func displaySettingLabel(s definition.Setting) string {
	if s.Label != "" {
		return s.Label
	}
	return s.Name
}

func displaySettingValue(s definition.Setting, v string) string {
	if s.Type == "password" {
		return "*****"
	}
	return v
}
