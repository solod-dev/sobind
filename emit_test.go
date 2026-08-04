package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBind(t *testing.T) {
	srcDir := "testdata/src"
	dstDir := "testdata/dst"

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatal(err)
	}

	var found int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".h") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".h")
		t.Run(name, func(t *testing.T) {
			srcPath := filepath.Join(srcDir, e.Name())
			dstPath := filepath.Join(dstDir, name+".go")

			compare(t, srcPath, dstPath, Options{Package: "main"})
		})
		found++
	}

	if found == 0 {
		t.Fatal("no .h files found in", srcDir)
	}
}

// TestBindBody checks the function bodies, which are off by default.
func TestBindBody(t *testing.T) {
	srcPath := filepath.Join("testdata/src", "funcs.h")
	dstPath := filepath.Join("testdata/dst", "funcs_body.go")
	compare(t, srcPath, dstPath, Options{Package: "main", Body: true})
}

// compare emits the header and diffs it against the expected output.
func compare(t *testing.T, srcPath, dstPath string, opts Options) {
	t.Helper()

	got, err := Emit([]string{srcPath}, opts)
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("missing expected output %s: %v", dstPath, err)
	}

	if string(got) != string(want) {
		t.Errorf("output mismatch for %s\n--- want\n%s\n--- got\n%s", srcPath, want, got)
	}
}
