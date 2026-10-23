package service

import (
	"os"
	"path/filepath"
)

// LocalDefsDir returns the repo-local definitions directory: first
// ./definitions-local relative to the working directory, then relative to
// the executable (so an installed binary still finds the repo's local defs).
// Empty string when neither exists.
func LocalDefsDir() string {
	if wd, err := os.Getwd(); err == nil {
		dir := filepath.Join(wd, "definitions-local")
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			return dir
		}
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "definitions-local")
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}
