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
	Package  string   // Go package name
	Includes []string // extra directories to search for included headers
	Body     bool     // emit function bodies instead of declarations only
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

	g := &generator{
		opts:      opts,
		allowed:   allowed,
		structs:   make(map[string]*structDecl),
		enums:     make(map[string]bool),
		consts:    make(map[string]constDecl),
		funcs:     make(map[string]*funcDecl),
		vars:      make(map[string]*varDecl),
		funcTypes: make(map[string]funcTypeDecl),
	}

	g.walkAST(ast)
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
	allowed   map[string]bool
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
	return g.allowed[abs]
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
					g.vars[name] = &varDecl{name: name, typ: goType, order: g.nextOrder()}
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
					g.funcTypes[name] = funcTypeDecl{name: name, sig: sig, order: g.nextOrder()}
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
	sd := &structDecl{name: name, cname: externName(name, "struct", st.Typedef()), order: g.nextOrder()}
	g.structs[name] = sd
	if st.HasFlexibleArrayMember() {
		return // opaque
	}
	sd.fields = g.mapFields(st)
}

func (g *generator) addUnion(name string, ut *cc.UnionType) {
	if _, exists := g.structs[name]; exists {
		return
	}

	sd := &structDecl{name: name, cname: externName(name, "union", ut.Typedef()), order: g.nextOrder()}
	g.structs[name] = sd
	sd.fields = g.mapFields(ut)
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
		name := cname
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
		g.consts[name] = constDecl{name: name, value: valStr, order: g.nextOrder()}
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
		name:     name,
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

		pos := m.Position().Filename
		if !g.isAllowed(pos) {
			continue
		}

		value, note := macroValue(m, macros)
		if value == "" && note == "" {
			continue
		}
		g.consts[name] = constDecl{name: name, value: value, note: note, order: g.nextOrder()}
	}
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
			buf.WriteString(c.name)
			buf.WriteString(": ")
			buf.WriteString(c.note)
			buf.WriteString("\n")
			continue
		}
		buf.WriteString("\n//so:extern ")
		buf.WriteString(c.name)
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
		buf.WriteString(v.name)
		buf.WriteString("\nvar ")
		buf.WriteString(v.name)
		buf.WriteString(" ")
		buf.WriteString(v.typ)
		buf.WriteString("\n")
	}
}

func (g *generator) emitFuncs(buf *strings.Builder) {
	for _, f := range sorted(g.funcs) {
		buf.WriteString("\n//so:extern ")
		buf.WriteString(f.name)
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
