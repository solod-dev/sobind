package main

// structDecl is a C struct or union. Both map to a Go struct: an extern type
// only names the C layout, it does not define it.
type structDecl struct {
	name   string
	fields []fieldDecl // nil means opaque
	order  int
}

type fieldDecl struct {
	name  string
	cname string // C name, differs from name when the C name is a Go keyword
	typ   string
}

type constDecl struct {
	name  string
	value string
	order int
}

type funcDecl struct {
	name     string
	params   []paramDecl
	result   string
	variadic bool
	order    int
}

type paramDecl struct {
	name string
	typ  string
}

type varDecl struct {
	name  string
	typ   string
	order int
}
