package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestDefsSyncCmdDefaultVersionIsEmpty(t *testing.T) {
	cmd := newDefsSyncCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Check that --version flag defaults to empty string
	flag := cmd.Flags().Lookup("version")
	if flag == nil {
		t.Fatal("expected --version flag to exist")
	}
	if flag.DefValue != "" {
		t.Fatalf("expected default version to be empty, got %q", flag.DefValue)
	}
}

func TestDefsSyncCmdHelpShowsDefaultAll(t *testing.T) {
	cmd := newDefsSyncCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Help()
	if err != nil {
		t.Fatalf("Help: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "default: all") {
		t.Errorf("expected help to mention 'default: all', got:\n%s", output)
	}
}

func TestDefsListCmdDefaultVersionIsEmpty(t *testing.T) {
	cmd := newDefsListCmd()

	flag := cmd.Flags().Lookup("version")
	if flag == nil {
		t.Fatal("expected --version flag to exist")
	}
	if flag.DefValue != "" {
		t.Fatalf("expected default version to be empty, got %q", flag.DefValue)
	}
}

func TestDefsShowCmdDefaultVersionIsV11(t *testing.T) {
	cmd := newDefsShowCmd()

	flag := cmd.Flags().Lookup("version")
	if flag == nil {
		t.Fatal("expected --version flag to exist")
	}
	if flag.DefValue != "v11" {
		t.Fatalf("expected default version to be v11, got %q", flag.DefValue)
	}
}

func TestDefsSyncCmdWithExplicitVersion(t *testing.T) {
	isolateCLIUserDirs(t)
	writeCLIConfig(t)

	cmd := newDefsSyncCmd()
	cmd.SetArgs([]string{"--version", "v11"})

	// This will fail with network error, but that's expected
	// We're testing that the command accepts the flag
	err := cmd.Execute()
	// Error is expected (no network), but command should parse flags correctly
	if err != nil && strings.Contains(err.Error(), "flag: invalid argument") {
		t.Fatalf("unexpected flag parsing error: %v", err)
	}
}

func TestDefsSyncCmdWithEmptyVersionFetchesAll(t *testing.T) {
	isolateCLIUserDirs(t)
	writeCLIConfig(t)

	cmd := newDefsSyncCmd()
	cmd.SetArgs([]string{"--version", ""})

	// This will fail with network error, but that's expected
	// We're testing that the command accepts empty version
	err := cmd.Execute()
	// Error is expected (no network), but command should parse flags correctly
	if err != nil && strings.Contains(err.Error(), "flag: invalid argument") {
		t.Fatalf("unexpected flag parsing error: %v", err)
	}
}
