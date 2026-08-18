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

func TestBindBody(t *testing.T) {
	srcPath := filepath.Join("testdata/src", "funcs.h")
	dstPath := filepath.Join("testdata/dst", "funcs_body.go")
	compare(t, srcPath, dstPath, Options{Package: "main", Body: true})
}

func TestBindStyle(t *testing.T) {
	// One prefix covers uv_ and UV_, because the match ignores case.
	srcPath := filepath.Join("testdata/src", "style.h")
	dstPath := filepath.Join("testdata/dst", "style_go.go")
	opts := Options{Package: "main", Style: styleGo, Strip: []string{"uv_"}}
	compare(t, srcPath, dstPath, opts)
}

func TestBindScope(t *testing.T) {
	// -scope emits the headers under a directory, so an umbrella header that
	// only includes them produces a binding. The default TestBind case covers
	// the same header without -scope, where it emits nothing.
	srcPath := filepath.Join("testdata/src", "umbrella.h")
	dstPath := filepath.Join("testdata/dst", "umbrella_scope.go")
	opts := Options{Package: "main", Scope: []string{"testdata/src/umbrella"}}
	compare(t, srcPath, dstPath, opts)
}

func TestBindCollision(t *testing.T) {
	// Check that two C names mapping to one So name fail.
	srcPath := filepath.Join("testdata/src", "collide.h")
	opts := Options{Package: "main", Style: styleGo, Strip: []string{"uv_"}}
	_, err := Emit([]string{srcPath}, opts)
	if err == nil {
		t.Fatal("expected a name collision error")
	}
	want := "C names collide; copy these into a -rename file and edit the So names apart:" +
		"\nUV_TIMEOUT  Timeout\nuv_timeout  Timeout"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err, want)
	}
}

func TestBindRename(t *testing.T) {
	// A rename file gives distinct So names to the two colliding symbols.
	rename, err := parseRenames(filepath.Join("testdata/src", "collide.rename"))
	if err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join("testdata/src", "collide.h")
	dstPath := filepath.Join("testdata/dst", "collide_rename.go")
	opts := Options{Package: "main", Style: styleGo, Strip: []string{"uv_"}, Rename: rename}
	compare(t, srcPath, dstPath, opts)
}

func TestBindExclude(t *testing.T) {
	// A rename file with a bare C name drops that symbol, so the pair no
	// longer collides.
	rename, err := parseRenames(filepath.Join("testdata/src", "collide_exclude.rename"))
	if err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join("testdata/src", "collide.h")
	dstPath := filepath.Join("testdata/dst", "collide_exclude.go")
	opts := Options{Package: "main", Style: styleGo, Strip: []string{"uv_"}, Rename: rename}
	compare(t, srcPath, dstPath, opts)
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
