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

			got, err := Emit([]string{srcPath}, "main")
			if err != nil {
				t.Fatal(err)
			}

			want, err := os.ReadFile(dstPath)
			if err != nil {
				t.Fatalf("missing expected output %s: %v", dstPath, err)
			}

			if string(got) != string(want) {
				t.Errorf("output mismatch for %s\n--- want\n%s\n--- got\n%s", e.Name(), want, got)
			}
		})
		found++
	}

	if found == 0 {
		t.Fatal("no .h files found in", srcDir)
	}
}
