package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// nameStyle selects the So names sobind gives to C symbols.
type nameStyle int

const (
	// styleC keeps the C name. Most C names are lower case, so the symbol is
	// unexported and only reachable inside the generated package.
	styleC nameStyle = iota
	// styleGo maps the C name to an exported CamelCase name.
	styleGo
)

// parseStyle returns the name style with the given flag value.
func parseStyle(s string) (nameStyle, error) {
	switch s {
	case "c":
		return styleC, nil
	case "go":
		return styleGo, nil
	}
	return 0, fmt.Errorf("unknown name style %q, want c or go", s)
}

// renamer maps a C symbol name to the So name of the symbol.
type renamer struct {
	style nameStyle
	strip []string // C name prefixes to remove, longest first
}

func newRenamer(style nameStyle, strip []string) *renamer {
	prefixes := slices.Clone(strip)
	// The longest prefix wins, so sqlite3session_ goes before sqlite3_.
	slices.SortFunc(prefixes, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	return &renamer{style: style, strip: prefixes}
}

// name returns the So name for a C symbol name.
func (r *renamer) name(cname string) string {
	if r.style == styleC {
		return cname
	}
	// A removed prefix can leave a name that starts with a digit, as in
	// SDL_3DNOW. Such a name keeps the prefix.
	if name := camel(dropTypeSuffix(r.cut(cname))); isExported(name) {
		return name
	}
	return camel(dropTypeSuffix(cname))
}

// cut removes the first matching prefix from a C name. The match ignores case.
func (r *renamer) cut(cname string) string {
	for _, prefix := range r.strip {
		if len(cname) < len(prefix) {
			continue
		}
		if strings.EqualFold(cname[:len(prefix)], prefix) {
			return cname[len(prefix):]
		}
	}
	return cname
}

// dropTypeSuffix removes the C type suffix _t: uv_loop_t becomes uv_loop.
func dropTypeSuffix(cname string) string {
	name, ok := strings.CutSuffix(cname, "_t")
	if ok && name != "" {
		return name
	}
	return cname
}

// camel joins the underscore separated parts of a C name into one CamelCase
// name. A part in upper case only is lowered first, so SQLITE_OK becomes
// SqliteOk. Every other part keeps its inner capitals, so pMethods becomes
// PMethods.
func camel(cname string) string {
	var buf strings.Builder
	for part := range strings.SplitSeq(cname, "_") {
		if part == "" {
			continue
		}
		if isUpper(part) {
			part = strings.ToLower(part)
		}
		first, size := utf8.DecodeRuneInString(part)
		buf.WriteRune(unicode.ToUpper(first))
		buf.WriteString(part[size:])
	}
	return buf.String()
}

// isUpper reports whether a part of a C name has upper case letters only.
func isUpper(part string) bool {
	return part == strings.ToUpper(part) && part != strings.ToLower(part)
}

// isExported reports whether other packages can use a name.
func isExported(name string) bool {
	first, _ := utf8.DecodeRuneInString(name)
	return name != "" && unicode.IsUpper(first)
}
