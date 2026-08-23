package main

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	cc "modernc.org/cc/v4"
)

// Options control the emitted Go source.
type Options struct {
	Package  string            // Go package name
	Includes []string          // extra directories to search for included headers
	Scope    []string          // directories whose headers are emitted, beyond the named files
	Body     bool              // emit function bodies instead of declarations only
	Style    nameStyle         // symbol naming style
	Strip    []string          // C name prefixes to remove, Go naming style only
	Rename   map[string]string // C name to So name, overrides the style; empty So name drops the symbol
}

// Emit parses C header files and returns a Go source file with so:extern declarations.
func Emit(paths []string, opts Options) ([]byte, error) {
	cfg, err := cc.NewConfig("", "")
	if err != nil {
		return nil, fmt.Errorf("cc config: %w", err)
	}
	cfg.Header = true
	cfg.EvalAllMacros = true
	// Headers of a library are often included by their install path
	// (<SDL3/SDL_atomic.h>), so search the extra directories first.
	cfg.IncludePaths = append(slices.Clone(opts.Includes), cfg.IncludePaths...)
	cfg.SysIncludePaths = append(slices.Clone(opts.Includes), cfg.SysIncludePaths...)

	sources := []cc.Source{
		{Name: "<predefined>", Value: cfg.Predefined},
		{Name: "<builtin>", Value: cc.Builtin},
	}
	for _, p := range paths {
		sources = append(sources, cc.Source{Name: p})
	}

	ast, err := cc.Translate(cfg, sources)
	if err != nil {
		return nil, fmt.Errorf("cc translate: %w", err)
	}

	allowed := make(map[string]bool, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		allowed[abs] = true
	}

	// A scope directory widens emission past the named files to every header
	// under it, so an umbrella header that only includes the library's own
	// headers still produces a binding.
	scope := make([]string, 0, len(opts.Scope))
	for _, s := range opts.Scope {
		abs, err := filepath.Abs(s)
		if err != nil {
			return nil, err
		}
		scope = append(scope, abs)
	}

	g := &generator{
		opts:      opts,
		rename:    newRenamer(opts.Style, opts.Strip, opts.Rename),
		allowed:   allowed,
		scope:     scope,
		structs:   make(map[string]*structDecl),
		enums:     make(map[string]bool),
		consts:    make(map[string]constDecl),
		funcs:     make(map[string]*funcDecl),
		vars:      make(map[string]*varDecl),
		funcTypes: make(map[string]funcTypeDecl),
	}

	g.walkAST(ast)
	g.dropExcluded()
	if err := g.checkNames(); err != nil {
		return nil, err
	}
	return g.emit(), nil
}

var goReserved = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true,
	"continue": true, "default": true, "defer": true, "else": true,
	"fallthrough": true, "for": true, "func": true, "go": true,
	"goto": true, "if": true, "import": true, "interface": true,
	"map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true,
	"var": true,
}

type generator struct {
	opts      Options
	rename    *renamer
	allowed   map[string]bool
	scope     []string
	structs   map[string]*structDecl
	enums     map[string]bool
	consts    map[string]constDecl
	funcs     map[string]*funcDecl
	vars      map[string]*varDecl
	funcTypes map[string]funcTypeDecl
	order     int
}

func (g *generator) nextOrder() int {
	g.order++
	return g.order
}

func (g *generator) isAllowed(pos string) bool {
	abs, err := filepath.Abs(pos)
	if err != nil {
		return false
	}
	if g.allowed[abs] {
		return true
	}
	for _, dir := range g.scope {
		if underDir(dir, abs) {
			return true
		}
	}
	return false
}

// underDir reports whether path is dir itself or a file beneath it.
func underDir(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// isAggregateAllowed reports whether a struct or union belongs to the binding.
// Types declared by the allowed headers are written out in full, types from
// other headers (like FILE from stdio.h) stay opaque.
func (g *generator) isAggregateAllowed(td *cc.Declarator, tag cc.Token) bool {
	if td != nil && g.isAllowed(td.Position().Filename) {
		return true
	}
	return tag.SrcStr() != "" && g.isAllowed(tag.Position().Filename)
}

func (g *generator) walkAST(ast *cc.AST) {
	for tu := ast.TranslationUnit; tu != nil; tu = tu.TranslationUnit {
		ed := tu.ExternalDeclaration
		switch ed.Case {
		case cc.ExternalDeclarationDecl:
			g.walkDeclaration(ed.Declaration)
		case cc.ExternalDeclarationFuncDef:
			g.walkFuncDef(ed.FunctionDefinition)
		}
	}
	g.walkMacros(ast.Macros)
}

func (g *generator) walkDeclaration(decl *cc.Declaration) {
	if decl == nil || decl.Case != cc.DeclarationDecl {
		return
	}

	// Check for standalone struct/enum typedefs or tags with no declarators.
	if decl.InitDeclaratorList == nil {
		// Could be a bare "struct Foo { ... };" or "enum Color { ... };"
		g.walkDeclSpecTypes(decl.DeclarationSpecifiers)
		return
	}

	for idl := decl.InitDeclaratorList; idl != nil; idl = idl.InitDeclaratorList {
		id := idl.InitDeclarator
		if id == nil || id.Declarator == nil {
			continue
		}
		d := id.Declarator

		if d.IsStatic() || d.IsInline() {
			continue
		}

		pos := d.Position().Filename
		if !g.isAllowed(pos) {
			continue
		}

		if d.IsTypename() {
			g.walkTypedef(d)
			continue
		}

		name := d.Name()
		if name == "" {
			continue
		}

		t := d.Type()
		if ft, ok := t.(*cc.FunctionType); ok {
			g.addFunc(name, ft)
		} else {
			goType := g.mapType(t)
			if goType != "" {
				if _, exists := g.vars[name]; !exists {
					g.vars[name] = &varDecl{
						name:  g.rename.name(name),
						cname: name,
						typ:   goType,
						order: g.nextOrder(),
					}
				}
			}
		}
	}
}

func (g *generator) walkDeclSpecTypes(ds *cc.DeclarationSpecifiers) {
	if ds == nil {
		return
	}

	pos := ds.Position().Filename
	if !g.isAllowed(pos) {
		return
	}

	t := ds.Type()
	if t == nil {
		return
	}

	switch ut := t.(type) {
	case *cc.StructType:
		tag := ut.Tag()
		if tag.SrcStr() != "" {
			g.addStruct(tag.SrcStr(), ut)
		}
	case *cc.UnionType:
		tag := ut.Tag()
		if tag.SrcStr() != "" {
			g.addUnion(tag.SrcStr(), ut)
		}
	case *cc.EnumType:
		tag := ut.Tag()
		name := tag.SrcStr()
		if name == "" {
			name = "<anon>"
		}
		if !g.enums[name] {
			g.enums[name] = true
			g.addEnumConsts(ut)
		}
	}
}

func (g *generator) walkTypedef(d *cc.Declarator) {
	name := d.Name()
	if name == "" || strings.HasPrefix(name, "_") {
		return
	}

	t := d.Type()

	switch ut := t.(type) {
	case *cc.StructType:
		g.addStruct(name, ut)
	case *cc.UnionType:
		g.addUnion(name, ut)
	case *cc.EnumType:
		if !g.enums[name] {
			g.enums[name] = true
			g.addEnumConsts(ut)
		}
	case *cc.PointerType:
		// Function pointer typedefs like typedef int (*callback)(void*,int).
		elem := ut.Elem()
		if elem.Kind() == cc.Function {
			if ft, ok := elem.(*cc.FunctionType); ok {
				// mapFuncPtrType registers the types it walks, so call it
				// even when the typedef is already known.
				sig := g.mapFuncPtrType(ft)
				if _, exists := g.funcTypes[name]; !exists {
					g.funcTypes[name] = funcTypeDecl{
						name:  g.rename.name(name),
						cname: name,
						sig:   sig,
						order: g.nextOrder(),
					}
				}
			}
			return
		}
		// Other pointer typedefs like typedef struct Foo* FooRef - skip,
		// the struct itself will be handled separately.
	}
}

func (g *generator) addStruct(name string, st *cc.StructType) {
	if _, exists := g.structs[name]; exists {
		return
	}

	// Register before resolving fields to break self-referential cycles.
	sd := &structDecl{
		name:  g.rename.name(name),
		cname: externName(name, "struct", st.Typedef()),
		order: g.nextOrder(),
	}
	g.structs[name] = sd
	if st.HasFlexibleArrayMember() || !g.isAggregateAllowed(st.Typedef(), st.Tag()) {
		return // opaque
	}
	sd.fields = g.mapFields(st)
}

func (g *generator) addUnion(name string, ut *cc.UnionType) {
	if _, exists := g.structs[name]; exists {
		return
	}

	sd := &structDecl{
		name:  g.rename.name(name),
		cname: externName(name, "union", ut.Typedef()),
		order: g.nextOrder(),
	}
	g.structs[name] = sd
	if !g.isAggregateAllowed(ut.Typedef(), ut.Tag()) {
		return // opaque
	}
	sd.fields = g.mapFields(ut)
}

// addConstStruct registers the const twin of a struct and returns its So name.
// The twin repeats the fields of the base struct under the C name with a const
// modifier. A So function passed as a C callback needs the twin to keep the
// const of a pointer parameter.
func (g *generator) addConstStruct(name string, st *cc.StructType) string {
	key := constKey(name)
	if sd, exists := g.structs[key]; exists {
		return sd.name
	}

	// Register before resolving fields to break self-referential cycles.
	sd := &structDecl{
		name:  g.rename.constName(name),
		cname: "const " + externName(name, "struct", st.Typedef()),
		order: g.nextOrder(),
	}
	g.structs[key] = sd
	if st.HasFlexibleArrayMember() || !g.isAggregateAllowed(st.Typedef(), st.Tag()) {
		return sd.name // opaque
	}
	sd.fields = g.mapFields(st)
	return sd.name
}

// addConstUnion registers the const twin of a union and returns its So name.
func (g *generator) addConstUnion(name string, ut *cc.UnionType) string {
	key := constKey(name)
	if sd, exists := g.structs[key]; exists {
		return sd.name
	}

	sd := &structDecl{
		name:  g.rename.constName(name),
		cname: "const " + externName(name, "union", ut.Typedef()),
		order: g.nextOrder(),
	}
	g.structs[key] = sd
	if !g.isAggregateAllowed(ut.Typedef(), ut.Tag()) {
		return sd.name // opaque
	}
	sd.fields = g.mapFields(ut)
	return sd.name
}

// constKey returns the map key of a const twin. The base type holds the plain
// C name, so the twin needs a key of its own.
func constKey(name string) string {
	return "const " + name
}

// externName returns the C name of a struct or union. A typedef name stands on
// its own, a tag needs the "struct" or "union" keyword at every use in C.
func externName(name, keyword string, td *cc.Declarator) string {
	if td != nil && td.Name() == name {
		return name
	}
	return keyword + " " + name
}

// aggregate is the part of the C struct and union APIs used for field mapping.
type aggregate interface {
	IsIncomplete() bool
	NumFields() int
	FieldByIndex(i int) *cc.Field
}

func (g *generator) mapFields(at aggregate) []fieldDecl {
	if at.IsIncomplete() {
		return nil
	}

	n := at.NumFields()
	var fields []fieldDecl
	for i := range n {
		f := at.FieldByIndex(i)
		if f.IsBitfield() {
			return nil // opaque
		}
		cname := f.Name()
		if cname == "" {
			return nil // anonymous field - opaque
		}
		goType := g.mapType(f.Type())
		if goType == "" {
			return nil // unmappable - opaque
		}
		name := g.rename.name(cname)
		if goReserved[name] {
			// A `c:"..."` tag keeps the C name when the Go one has to change.
			name += "_"
		}
		fields = append(fields, fieldDecl{name: name, cname: cname, typ: goType})
	}
	return fields
}

func (g *generator) addEnumConsts(et *cc.EnumType) {
	for _, e := range et.Enumerators() {
		name := e.Token.SrcStr()
		if strings.HasPrefix(name, "_") {
			continue
		}
		if _, exists := g.consts[name]; exists {
			continue
		}
		val := e.Value()
		valStr := formatValue(val)
		if valStr == "" {
			continue
		}
		g.consts[name] = constDecl{
			name:  g.rename.name(name),
			cname: name,
			value: valStr,
			order: g.nextOrder(),
		}
	}
}

func formatValue(v cc.Value) string {
	switch v := v.(type) {
	case cc.Int64Value:
		return fmt.Sprintf("%d", int64(v))
	case cc.UInt64Value:
		return fmt.Sprintf("%d", uint64(v))
	case cc.StringValue:
		return fmt.Sprintf("%q", string(v))
	default:
		return ""
	}
}

func (g *generator) addFunc(name string, ft *cc.FunctionType) {
	if _, exists := g.funcs[name]; exists {
		return
	}
	if strings.HasPrefix(name, "_") {
		return
	}

	params := g.mapParams(ft)
	result := ""
	rt := ft.Result()
	if rt != nil && rt.Kind() != cc.Void {
		result = g.mapType(rt)
	}

	g.funcs[name] = &funcDecl{
		name:     g.rename.name(name),
		cname:    name,
		params:   params,
		result:   result,
		variadic: ft.IsVariadic(),
		order:    g.nextOrder(),
	}
}

func (g *generator) mapParams(ft *cc.FunctionType) []paramDecl {
	ccParams := ft.Parameters()
	if len(ccParams) == 1 && ccParams[0].Type().Kind() == cc.Void {
		return nil
	}

	var params []paramDecl
	for i, p := range ccParams {
		name := p.Name()
		if name == "" {
			name = fmt.Sprintf("arg%d", i)
		}
		if goReserved[name] {
			name += "_"
		}
		goType := g.mapType(p.Type())
		if goType == "" {
			goType = "any"
		}
		// Only direct function params get the string conversion;
		// nested pointers (const char**) stay as **c.ConstChar.
		if goType == "*c.ConstChar" {
			goType = "string"
		}
		params = append(params, paramDecl{name: name, typ: goType})
	}
	return params
}

func (g *generator) walkFuncDef(fd *cc.FunctionDefinition) {
	if fd == nil || fd.Declarator == nil {
		return
	}
	d := fd.Declarator
	if d.IsStatic() || d.IsInline() {
		return
	}
	pos := d.Position().Filename
	if !g.isAllowed(pos) {
		return
	}
	name := d.Name()
	if name == "" || strings.HasPrefix(name, "_") {
		return
	}

	t := d.Type()
	ft, ok := t.(*cc.FunctionType)
	if !ok {
		return
	}
	g.addFunc(name, ft)
}

func (g *generator) walkMacros(macros map[string]*cc.Macro) {
	// Sort macro names for deterministic output order.
	names := slices.Sorted(maps.Keys(macros))
	for _, name := range names {
		m := macros[name]
		if m.IsFnLike {
			continue
		}
		if strings.HasPrefix(name, "_") {
			continue
		}
		if _, exists := g.consts[name]; exists {
			continue
		}
		if _, exists := g.vars[name]; exists {
			continue
		}

		pos := m.Position().Filename
		if !g.isAllowed(pos) {
			continue
		}

		kind, value, note := macroValue(m, macros)
		switch {
		case note != "":
			g.consts[name] = constDecl{
				name:  g.rename.name(name),
				cname: name,
				note:  note,
				order: g.nextOrder(),
			}
		case kind == macroString:
			// A string literal is a char array in C, not a So string, so the
			// macro names a C string pointer.
			g.vars[name] = &varDecl{
				name:    g.rename.name(name),
				cname:   name,
				typ:     "*c.ConstChar",
				comment: value,
				order:   g.nextOrder(),
			}
		case value != "":
			g.consts[name] = constDecl{
				name:  g.rename.name(name),
				cname: name,
				value: value,
				order: g.nextOrder(),
			}
		}
	}
}

// dropExcluded removes the symbols the rename file drops. A dropped symbol
// leaves the walk with a normal name, so this runs before checkNames, which
// then ignores it. The keys of every declaration map are C names.
func (g *generator) dropExcluded() {
	for _, cname := range g.rename.excluded() {
		delete(g.structs, cname)
		delete(g.structs, constKey(cname))
		delete(g.consts, cname)
		delete(g.funcs, cname)
		delete(g.vars, cname)
		delete(g.funcTypes, cname)
	}
}

// checkNames reports C symbols that map to one So name.
func (g *generator) checkNames() error {
	var all []symbol
	all = append(all, symbols(g.funcTypes)...)
	all = append(all, symbols(g.structs)...)
	all = append(all, symbols(g.consts)...)
	all = append(all, symbols(g.vars)...)
	all = append(all, symbols(g.funcs)...)

	// So name to the C names that map to it, and the So names in first-seen
	// order for a stable report.
	cnames := map[string][]string{}
	var order []string
	for _, s := range all {
		cname, name := s.names()
		if _, ok := cnames[name]; !ok {
			order = append(order, name)
		}
		cnames[name] = append(cnames[name], cname)
	}

	// Collect the colliding pairs and the width of the C name column so the So
	// names line up.
	type pair struct{ cname, soname string }
	var pairs []pair
	width := 0
	for _, name := range order {
		if len(cnames[name]) < 2 {
			continue
		}
		for _, cname := range cnames[name] {
			pairs = append(pairs, pair{cname, name})
			width = max(width, len(cname))
		}
	}
	if len(pairs) == 0 {
		return nil
	}

	var buf strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&buf, "\n%-*s  %s", width, p.cname, p.soname)
	}
	return fmt.Errorf("C names collide; copy these into a -rename file and edit the So names apart:%s", buf.String())
}

func (g *generator) emit() []byte {
	var buf strings.Builder
	g.emitHeader(&buf)
	g.emitFuncTypes(&buf)
	g.emitStructs(&buf)
	g.emitConsts(&buf)
	g.emitVars(&buf)
	g.emitFuncs(&buf)
	return []byte(buf.String())
}

func (g *generator) emitHeader(buf *strings.Builder) {
	buf.WriteString(`// Code generated by "so bind"; DO NOT EDIT.`)
	buf.WriteString("\n\n")
	buf.WriteString("package ")
	buf.WriteString(g.opts.Package)
	buf.WriteString("\n\n")

	buf.WriteString(`import "solod.dev/so/c"`)
	buf.WriteString("\n")
	buf.WriteString("var _ c.Char\n")
}

func (g *generator) emitFuncTypes(buf *strings.Builder) {
	for _, t := range sorted(g.funcTypes) {
		buf.WriteString("\ntype ")
		buf.WriteString(t.name)
		buf.WriteString(" ")
		buf.WriteString(t.sig)
		buf.WriteString("\n")
	}
}

func (g *generator) emitStructs(buf *strings.Builder) {
	for _, s := range sorted(g.structs) {
		buf.WriteString("\n//so:extern ")
		buf.WriteString(s.cname)
		buf.WriteString("\ntype ")
		buf.WriteString(s.name)
		buf.WriteString(" struct {")
		if s.fields != nil {
			buf.WriteString("\n")
			for _, f := range s.fields {
				buf.WriteString("\t")
				buf.WriteString(f.name)
				buf.WriteString(" ")
				buf.WriteString(f.typ)
				if f.cname != f.name {
					fmt.Fprintf(buf, " `c:%q`", f.cname)
				}
				buf.WriteString("\n")
			}
		}
		buf.WriteString("}\n")
	}
}

func (g *generator) emitConsts(buf *strings.Builder) {
	for _, c := range sorted(g.consts) {
		if c.value == "" {
			buf.WriteString("\n// ")
			buf.WriteString(c.cname)
			buf.WriteString(": ")
			buf.WriteString(c.note)
			buf.WriteString("\n")
			continue
		}
		buf.WriteString("\n//so:extern ")
		buf.WriteString(c.cname)
		buf.WriteString("\nconst ")
		buf.WriteString(c.name)
		buf.WriteString(" = ")
		buf.WriteString(c.value)
		buf.WriteString("\n")
	}
}

func (g *generator) emitVars(buf *strings.Builder) {
	for _, v := range sorted(g.vars) {
		buf.WriteString("\n//so:extern ")
		buf.WriteString(v.cname)
		buf.WriteString("\nvar ")
		buf.WriteString(v.name)
		buf.WriteString(" ")
		buf.WriteString(v.typ)
		if v.comment != "" {
			buf.WriteString(" // ")
			buf.WriteString(v.comment)
		}
		buf.WriteString("\n")
	}
}

func (g *generator) emitFuncs(buf *strings.Builder) {
	for _, f := range sorted(g.funcs) {
		buf.WriteString("\n//so:extern ")
		buf.WriteString(f.cname)
		buf.WriteString("\nfunc ")
		buf.WriteString(f.name)
		buf.WriteString("(")
		for i, p := range f.params {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(p.name)
			buf.WriteString(" ")
			buf.WriteString(p.typ)
		}
		if f.variadic {
			if len(f.params) > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString("args ...any")
		}
		buf.WriteString(")")
		if f.result != "" {
			buf.WriteString(" ")
			buf.WriteString(f.result)
		}
		if g.opts.Body {
			g.emitFuncBody(buf, f)
		} else {
			buf.WriteString("\n")
		}
	}
}

func (g *generator) emitFuncBody(buf *strings.Builder, f *funcDecl) {
	buf.WriteString(" {\n")
	// Blank-assign all parameters.
	allParams := make([]string, 0, len(f.params))
	for _, p := range f.params {
		allParams = append(allParams, p.name)
	}
	if f.variadic {
		allParams = append(allParams, "args")
	}
	if len(allParams) > 0 {
		blanks := make([]string, len(allParams))
		for i := range blanks {
			blanks[i] = "_"
		}
		buf.WriteString("\t")
		buf.WriteString(strings.Join(blanks, ", "))
		buf.WriteString(" = ")
		buf.WriteString(strings.Join(allParams, ", "))
		buf.WriteString("\n")
	}

	// Return zero value.
	if f.result != "" {
		buf.WriteString("\treturn ")
		buf.WriteString(zeroValue(f.result))
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
}
