package main

type structDecl struct {
	name   string
	fields []fieldDecl // nil means opaque
	order  int
}

type fieldDecl struct {
	name string
	typ  string
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
