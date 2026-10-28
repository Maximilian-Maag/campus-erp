// Package golden provides a small, dependency-free golden-file harness and an
// optional XML-schema check, shared by the interface tests (SEPA, XRechnung, …).
//
// A golden file pins the exact bytes a generator produces, so an unintended
// change is a test failure. Run the suite with UPDATE_GOLDEN=1 to rewrite the
// goldens intentionally.
package golden

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Compare checks got against the file at path (relative to the package
// directory, typically "testdata/<name>"). With UPDATE_GOLDEN=1 it writes the
// file instead. It returns an error rather than failing a test directly, so it
// is itself testable.
func Compare(path string, got []byte) error {
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("golden: mkdir: %w", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			return fmt.Errorf("golden: write: %w", err)
		}
		return nil
	}
	want, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("golden: read %s (run with UPDATE_GOLDEN=1 to create it): %w", path, err)
	}
	if string(got) != string(want) {
		return fmt.Errorf("golden: %s differs.\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
	return nil
}

// Assert applies Compare and fails the test on any error.
func Assert(t *testing.T, path string, got []byte) {
	t.Helper()
	if err := Compare(path, got); err != nil {
		t.Fatal(err)
	}
}

// ValidateXSD validates the given files against an XML schema with xmllint.
// If envKey is non-empty and set, its value is used as the schema path;
// otherwise xsdPath is used. The test is skipped when xmllint or the schema is
// unavailable, so the suite still runs where the (proprietary) schema is absent.
func ValidateXSD(t *testing.T, xsdPath, envKey string, files ...string) {
	t.Helper()
	if envKey != "" {
		if v := os.Getenv(envKey); v != "" {
			xsdPath = v
		}
	}
	if xsdPath == "" {
		t.Skip("no schema configured")
	}
	if _, err := os.Stat(xsdPath); err != nil {
		t.Skipf("schema %s not available", xsdPath)
	}
	if _, err := exec.LookPath("xmllint"); err != nil {
		t.Skip("xmllint not available")
	}
	args := append([]string{"--noout", "--schema", xsdPath}, files...)
	if out, err := exec.Command("xmllint", args...).CombinedOutput(); err != nil {
		t.Fatalf("schema validation failed: %v\n%s", err, out)
	}
}
