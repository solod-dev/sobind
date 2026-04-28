package main

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	cc "modernc.org/cc/v4"
)

// Emit parses C header files and returns a Go source file with so:extern declarations.
func Emit(paths []string, pkgName string) ([]byte, error) {
	cfg, err := cc.NewConfig("", "")
	if err != nil {
		return nil, fmt.Errorf("cc config: %w", err)
	}
	cfg.Header = true
	cfg.EvalAllMacros = true

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
		allowed:   allowed,
		structs:   make(map[string]*structDecl),
		enums:     make(map[string]bool),
		consts:    make(map[string]constDecl),
		funcs:     make(map[string]*funcDecl),
		vars:      make(map[string]*varDecl),
		funcTypes: make(map[string]string),
	}

	g.walkAST(ast)
	return g.emit(pkgName), nil
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
	allowed   map[string]bool
	structs   map[string]*structDecl
	enums     map[string]bool
	consts    map[string]constDecl
	funcs     map[string]*funcDecl
	vars      map[string]*varDecl
	funcTypes map[string]string // C type string -> Go type name
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
				sig := g.mapFuncPtrType(ft)
				if _, exists := g.funcTypes[name]; !exists {
					g.funcTypes[name] = fmt.Sprintf("type %s = %s", name, sig)
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
	sd := &structDecl{name: name, order: g.nextOrder()}
	g.structs[name] = sd
	sd.fields = g.mapStructFields(st)
}

func (g *generator) mapStructFields(st *cc.StructType) []fieldDecl {
	if st.IsIncomplete() {
		return nil
	}
	if st.HasFlexibleArrayMember() {
		return nil
	}

	n := st.NumFields()
	var fields []fieldDecl
	for i := 0; i < n; i++ {
		f := st.FieldByIndex(i)
		if f.IsBitfield() {
			return nil // opaque
		}
		fname := f.Name()
		if fname == "" {
			return nil // anonymous field - opaque
		}
		ft := f.Type()
		if ft.Kind() == cc.Union {
			return nil // contains union - opaque
		}
		goType := g.mapType(ft)
		if goType == "" {
			return nil // unmappable - opaque
		}
		fields = append(fields, fieldDecl{name: fname, typ: goType})
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
		// nested pointers (const char**) stay as **c.CChar.
		if goType == "*c.CChar" {
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

		valStr := macroValue(m)
		if valStr == "" {
			continue
		}
		g.consts[name] = constDecl{name: name, value: valStr, order: g.nextOrder()}
	}
}

func (g *generator) emit(pkgName string) []byte {
	var buf strings.Builder
	buf.WriteString(`// Code generated by "so bind"; DO NOT EDIT.`)
	buf.WriteString("\n\n")
	buf.WriteString("package ")
	buf.WriteString(pkgName)
	buf.WriteString("\n\n")

	buf.WriteString(`import "solod.dev/so/c"`)
	buf.WriteString("\n")
	buf.WriteString("var _ c.Char\n")

	// Collect and sort all func type declarations needed.
	var funcTypeDefs []string
	for _, def := range g.funcTypes {
		funcTypeDefs = append(funcTypeDefs, def)
	}
	sort.Strings(funcTypeDefs)
	for _, def := range funcTypeDefs {
		buf.WriteString("\n")
		buf.WriteString(def)
		buf.WriteString("\n")
	}

	// Structs sorted by order.
	structs := make([]*structDecl, 0, len(g.structs))
	for _, s := range g.structs {
		structs = append(structs, s)
	}
	sort.Slice(structs, func(i, j int) bool { return structs[i].order < structs[j].order })

	for _, s := range structs {
		buf.WriteString("\n//so:extern ")
		buf.WriteString(s.name)
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
				buf.WriteString("\n")
			}
		}
		buf.WriteString("}\n")
	}

	// Constants sorted by order.
	consts := make([]constDecl, 0, len(g.consts))
	for _, c := range g.consts {
		consts = append(consts, c)
	}
	sort.Slice(consts, func(i, j int) bool { return consts[i].order < consts[j].order })

	for _, c := range consts {
		buf.WriteString("\n//so:extern ")
		buf.WriteString(c.name)
		buf.WriteString("\nconst ")
		buf.WriteString(c.name)
		buf.WriteString(" = ")
		buf.WriteString(c.value)
		buf.WriteString("\n")
	}

	// Variables sorted by order.
	vars := make([]*varDecl, 0, len(g.vars))
	for _, v := range g.vars {
		vars = append(vars, v)
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].order < vars[j].order })

	for _, v := range vars {
		buf.WriteString("\n//so:extern ")
		buf.WriteString(v.name)
		buf.WriteString("\nvar ")
		buf.WriteString(v.name)
		buf.WriteString(" ")
		buf.WriteString(v.typ)
		buf.WriteString("\n")
	}

	// Functions sorted by order.
	funcs := make([]*funcDecl, 0, len(g.funcs))
	for _, f := range g.funcs {
		funcs = append(funcs, f)
	}
	sort.Slice(funcs, func(i, j int) bool { return funcs[i].order < funcs[j].order })

	for _, f := range funcs {
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
		buf.WriteString(" {\n")
		g.emitFuncBody(&buf, f)
		buf.WriteString("}\n")
	}

	return []byte(buf.String())
}

func (g *generator) emitFuncBody(buf *strings.Builder, f *funcDecl) {
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
}

// macroValue returns the formatted constant value for a macro, or "" if the
// macro doesn't represent a usable constant. This filters out macros that
// expand to keywords (like `#define SQLITE_EXTERN extern`).
func macroValue(m *cc.Macro) string {
	hasLiteral := false
	hasString := false
	for _, tok := range m.ReplacementList() {
		switch rune(tok.Ch) {
		case rune(cc.INTCONST), rune(cc.FLOATCONST), rune(cc.CHARCONST),
			rune(cc.LONGCHARCONST), rune(cc.PPNUMBER):
			hasLiteral = true
		case rune(cc.STRINGLITERAL), rune(cc.LONGSTRINGLITERAL):
			hasString = true
		}
	}

	if hasString {
		// For string macros, use the source text directly since the cc
		// library may return UnknownValue for string constants.
		tokens := m.ReplacementList()
		if len(tokens) == 1 {
			return tokens[0].SrcStr()
		}
		return ""
	}

	if !hasLiteral {
		return ""
	}

	return formatValue(m.Value())
}
