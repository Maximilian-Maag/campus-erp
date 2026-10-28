package golden

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareWriteThenMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "testdata", "sample.txt")
	os.Setenv("UPDATE_GOLDEN", "1")
	if err := Compare(path, []byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	os.Unsetenv("UPDATE_GOLDEN")

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("golden not written: %v", err)
	}
	if err := Compare(path, []byte("hello")); err != nil {
		t.Fatalf("match should pass: %v", err)
	}
}

func TestCompareMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "testdata", "s.txt")
	os.Setenv("UPDATE_GOLDEN", "1")
	_ = Compare(path, []byte("a"))
	os.Unsetenv("UPDATE_GOLDEN")

	if err := Compare(path, []byte("b")); err == nil {
		t.Error("mismatch must return an error")
	}
}

func TestCompareMissingFile(t *testing.T) {
	if err := Compare(filepath.Join(t.TempDir(), "nope.txt"), []byte("x")); err == nil {
		t.Error("missing golden must return an error")
	}
}

func TestAssertMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "testdata", "a.txt")
	os.Setenv("UPDATE_GOLDEN", "1")
	_ = Compare(path, []byte("ok"))
	os.Unsetenv("UPDATE_GOLDEN")
	Assert(t, path, []byte("ok"))
}

func TestValidateXSDNoSchemaIsSkipped(t *testing.T) {
	ValidateXSD(t, "", "", "does-not-matter.xml") // empty schema -> skip
}

func TestValidateXSDMissingSchemaSkips(t *testing.T) {
	ValidateXSD(t, filepath.Join(t.TempDir(), "absent.xsd"), "", "x.xml")
}

func TestValidateXSDMissingEnvSkips(t *testing.T) {
	ValidateXSD(t, "", "GOLDEN_TEST_XSD_UNSET", "x.xml")
}
