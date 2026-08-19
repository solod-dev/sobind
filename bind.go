package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	err := bind(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func bind(args []string) error {
	flags := flag.NewFlagSet("bind", flag.ContinueOnError)
	outFile := flags.String("o", "", "output file (default: stdout)")
	pkgName := flags.String("pkg", "main", "Go package name")
	var includes stringList
	flags.Var(&includes, "I", "include search directory (repeatable)")
	var scope stringList
	flags.Var(&scope, "scope", "directory whose headers are emitted, beyond the named files (repeatable)")
	body := flags.Bool("body", false, "emit function bodies (default: declaration only)")
	styleStr := flags.String("style", "c", "symbol naming: c (keep C names), cap (capitalized), or go (exported CamelCase)")
	var strip stringList
	flags.Var(&strip, "strip", "C name prefix to remove (repeatable)")
	renameFile := flags.String("rename", "", "file of 'cname soname' lines that set So names by hand")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return fmt.Errorf("usage: bind [-o output.go] [-pkg name] [-I dir] [-scope dir] [-body] " +
			"[-style c|go] [-strip prefix] [-rename file] <path>")
	}

	for _, s := range scope {
		info, err := os.Stat(s)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("-scope %s: not a directory", s)
		}
	}

	style, err := parseStyle(*styleStr)
	if err != nil {
		return err
	}

	rename, err := parseRenames(*renameFile)
	if err != nil {
		return err
	}

	paths, err := collectPaths(flags.Args())
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no header files found")
	}

	opts := Options{
		Package:  *pkgName,
		Includes: includes,
		Scope:    scope,
		Body:     *body,
		Style:    style,
		Strip:    strip,
		Rename:   rename,
	}
	out, err := Emit(paths, opts)
	if err != nil {
		return err
	}

	err = writeOutput(*outFile, out)
	return err
}

// parseRenames reads a rename file. A 'cname soname' line gives the C symbol
// cname the So name soname, verbatim. A 'cname' line on its own drops the
// symbol, recorded as an empty So name. Blank lines and lines starting with #
// are ignored. An empty path yields a nil map.
func parseRenames(path string) (map[string]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rename := map[string]string{}
	for i, line := range strings.Split(string(data), "\n") {
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) < 1 || len(fields) > 2 {
			return nil, fmt.Errorf("%s:%d: want 'cname' or 'cname soname', got %q", path, i+1, text)
		}
		cname := fields[0]
		if _, ok := rename[cname]; ok {
			return nil, fmt.Errorf("%s:%d: %s listed twice", path, i+1, cname)
		}
		if len(fields) == 2 {
			rename[cname] = fields[1]
		} else {
			rename[cname] = "" // drop the symbol
		}
	}
	return rename, nil
}

// stringList collects a repeatable flag.
type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ",")
}

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func collectPaths(args []string) ([]string, error) {
	var paths []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			entries, err := os.ReadDir(arg)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".h") {
					paths = append(paths, filepath.Join(arg, e.Name()))
				}
			}
		} else {
			paths = append(paths, arg)
		}
	}
	return paths, nil
}

func writeOutput(path string, out []byte) error {
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, out, 0o644)
	}
	_, err := os.Stdout.Write(out)
	return err
}
