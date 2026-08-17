package main

import (
	"cmp"
	"maps"
	"slices"
)

// ordered is a declaration that knows its position in the header.
type ordered interface {
	ordinal() int
}

// symbol is a declaration with a C name and a So name.
type symbol interface {
	names() (cname, name string)
}

// sorted returns the declarations in header order.
func sorted[T ordered](decls map[string]T) []T {
	s := slices.Collect(maps.Values(decls))
	slices.SortFunc(s, func(a, b T) int { return cmp.Compare(a.ordinal(), b.ordinal()) })
	return s
}

// symbols returns the declarations as symbols, in header order.
func symbols[T interface {
	ordered
	symbol
}](decls map[string]T) []symbol {
	s := sorted(decls)
	out := make([]symbol, len(s))
	for i, d := range s {
		out[i] = d
	}
	return out
}

// funcTypeDecl is a C function pointer typedef.
type funcTypeDecl struct {
	name  string
	cname string
	sig   string // Go function type, "func(c.Int) c.Int"
	order int
}

func (d funcTypeDecl) ordinal() int            { return d.order }
func (d funcTypeDecl) names() (string, string) { return d.cname, d.name }

// structDecl is a C struct or union. Both map to a Go struct: an extern type
// only names the C layout, it does not define it.
type structDecl struct {
	name   string
	cname  string      // C name, "struct Foo" for a type with no typedef
	fields []fieldDecl // nil means opaque
	order  int
}

func (d structDecl) ordinal() int            { return d.order }
func (d structDecl) names() (string, string) { return d.cname, d.name }

type fieldDecl struct {
	name  string
	cname string // C name, differs from name when the C name is a Go keyword
	typ   string
}

// constDecl is an enum member or an object-like macro. An empty value means
// the constant could not be expressed in So, and note says why.
type constDecl struct {
	name  string
	cname string
	value string
	note  string
	order int
}

func (d constDecl) ordinal() int            { return d.order }
func (d constDecl) names() (string, string) { return d.cname, d.name }

type funcDecl struct {
	name     string
	cname    string
	params   []paramDecl
	result   string
	variadic bool
	order    int
}

func (d funcDecl) ordinal() int            { return d.order }
func (d funcDecl) names() (string, string) { return d.cname, d.name }

type paramDecl struct {
	name string
	typ  string
}

type varDecl struct {
	name  string
	cname string
	typ   string
	order int
}

func (d varDecl) ordinal() int            { return d.order }
func (d varDecl) names() (string, string) { return d.cname, d.name }
