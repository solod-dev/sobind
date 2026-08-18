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
	body := flags.Bool("body", false, "emit function bodies (default: declaration only)")
	styleStr := flags.String("style", "c", "symbol naming: c (keep C names) or go (exported CamelCase)")
	var strip stringList
	flags.Var(&strip, "strip", "C name prefix to remove with -style=go (repeatable)")
	renameFile := flags.String("rename", "", "file of 'cname soname' lines that set So names by hand")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return fmt.Errorf("usage: bind [-o output.go] [-pkg name] [-I dir] [-body] " +
			"[-style c|go] [-strip prefix] [-rename file] <path>")
	}

	style, err := parseStyle(*styleStr)
	if err != nil {
		return err
	}
	if style != styleGo && len(strip) > 0 {
		return fmt.Errorf("-strip needs -style=go")
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

// parseRenames reads a rename file: one 'cname soname' pair per line, where
// soname is the So name to give the C symbol cname, verbatim. Blank lines and
// lines starting with # are ignored. An empty path yields a nil map.
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
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: want 'cname soname', got %q", path, i+1, text)
		}
		cname, soname := fields[0], fields[1]
		if prev, ok := rename[cname]; ok {
			return nil, fmt.Errorf("%s:%d: %s renamed twice, to %s and %s", path, i+1, cname, prev, soname)
		}
		rename[cname] = soname
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
