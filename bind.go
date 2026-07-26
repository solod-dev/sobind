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
	var includes pathList
	flags.Var(&includes, "I", "include search directory (repeatable)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return fmt.Errorf("usage: bind [-o output.go] [-pkg name] [-I dir] <path>")
	}

	paths, err := collectPaths(flags.Args())
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no header files found")
	}

	out, err := Emit(paths, *pkgName, includes)
	if err != nil {
		return err
	}

	err = writeOutput(*outFile, out)
	return err
}

// pathList collects a repeatable directory flag.
type pathList []string

func (l *pathList) String() string {
	return strings.Join(*l, ",")
}

func (l *pathList) Set(v string) error {
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
