package main

import (
	"fmt"
	"strings"

	cc "modernc.org/cc/v4"
)

// fixedWidth maps the fixed-width C typedefs to the So types of the same width.
var fixedWidth = map[string]string{
	"int8_t":    "int8",
	"int16_t":   "int16",
	"int32_t":   "int32",
	"int64_t":   "int64",
	"uint8_t":   "uint8",
	"uint16_t":  "uint16",
	"uint32_t":  "uint32",
	"uint64_t":  "uint64",
	"uintptr_t": "uintptr",
}

// namedTypes maps the C typedefs whose width follows the target to the so/c
// types of the same name. Everything else goes through the C type it is
// declared with, so the width stays whatever the C compiler picks.
var namedTypes = map[string]string{
	"size_t":    "c.Size",
	"ssize_t":   "c.SSize",
	"ptrdiff_t": "c.Ptrdiff",
	"intptr_t":  "c.Intptr",
}

func (g *generator) mapType(t cc.Type) string {
	name := typedefName(t)
	if goType, ok := fixedWidth[name]; ok {
		return goType
	}
	if goType, ok := namedTypes[name]; ok {
		return goType
	}

	switch t.Kind() {
	case cc.Void:
		return ""
	case cc.Bool:
		return "bool"
	case cc.Char:
		return "c.Char"
	case cc.SChar:
		return "c.SChar"
	case cc.UChar:
		return "c.UChar"
	case cc.Short:
		return "c.Short"
	case cc.UShort:
		return "c.UShort"
	case cc.Int:
		return "c.Int"
	case cc.UInt:
		return "c.UInt"
	case cc.Long:
		return "c.Long"
	case cc.ULong:
		return "c.ULong"
	case cc.LongLong:
		return "c.LongLong"
	case cc.ULongLong:
		return "c.ULongLong"
	case cc.Float:
		return "float32"
	case cc.Double:
		return "float64"
	case cc.LongDouble:
		return "c.LongDouble"
	case cc.Enum:
		// A C enum has the size of an int.
		return "c.Int"
	case cc.Ptr:
		return g.mapPointerType(t.(*cc.PointerType))
	case cc.Struct:
		return g.mapStructType(t.(*cc.StructType))
	case cc.Union:
		return g.mapUnionType(t.(*cc.UnionType))
	case cc.Function:
		typ, _ := g.mapFuncPtrType(t.(*cc.FunctionType))
		return typ
	case cc.Array:
		return g.mapArrayType(t.(*cc.ArrayType))
	default:
		return ""
	}
}

func (g *generator) mapPointerType(pt *cc.PointerType) string {
	elem := pt.Elem()
	if elem.Kind() == cc.Void {
		return "any"
	}
	// A pointer to a char type declared with a fixed-width typedef (uint8_t*)
	// is a byte buffer, mapped through the typedef below. A pointer to plain
	// char is a C string. signed char and unsigned char keep their signedness,
	// both because they are usually byte buffers rather than text and because
	// mapping them to plain char triggers a C pointer-sign warning at the call.
	if fixedWidth[typedefName(elem)] == "" {
		switch elem.Kind() {
		case cc.Char:
			if elem.Attributes().IsConst() {
				return "*c.ConstChar"
			}
			return "*c.Char"
		case cc.SChar:
			return "*c.SChar"
		case cc.UChar:
			return "*c.UChar"
		}
	}
	if elem.Kind() == cc.Function {
		ft, ok := elem.(*cc.FunctionType)
		if ok {
			typ, _ := g.mapFuncPtrType(ft)
			return typ
		}
	}
	inner := g.mapType(elem)
	if inner == "" {
		return "any"
	}
	return "*" + inner
}

func (g *generator) mapStructType(st *cc.StructType) string {
	name := aggregateName(st.Typedef(), st.Tag())
	if name == "" {
		return ""
	}
	// Ensure struct is registered.
	g.addStruct(name, st)
	return g.rename.name(name)
}

func (g *generator) mapUnionType(ut *cc.UnionType) string {
	name := aggregateName(ut.Typedef(), ut.Tag())
	if name == "" {
		return ""
	}
	// Ensure union is registered.
	g.addUnion(name, ut)
	return g.rename.name(name)
}

// mapFuncPtrType maps a C function pointer type to a Go function type. Unlike
// a plain function, a callback cannot fall back to the any type: C calls it
// through the signature it declares, so a parameter that does not match
// corrupts the call. A signature with a part sobind cannot map is unmappable as
// a whole, and reason says which part.
func (g *generator) mapFuncPtrType(ft *cc.FunctionType) (typ, reason string) {
	var sig strings.Builder
	sig.WriteString("func(")
	params := ft.Parameters()
	if len(params) == 1 && params[0].Type().Kind() == cc.Void {
		params = nil
	}
	for i, p := range params {
		if i > 0 {
			sig.WriteString(", ")
		}
		goType := g.mapConstType(p.Type())
		if goType == "" {
			return "", fmt.Sprintf("param %s has an unmappable type", paramName(p, i))
		}
		sig.WriteString(goType)
	}
	if ft.IsVariadic() {
		if len(params) > 0 {
			sig.WriteString(", ")
		}
		sig.WriteString("...any")
	}
	sig.WriteString(")")
	rt := ft.Result()
	if rt != nil && rt.Kind() != cc.Void {
		goType := g.mapType(rt)
		if goType == "" {
			return "", "the result type is unmappable"
		}
		sig.WriteString(" ")
		sig.WriteString(goType)
	}
	return sig.String(), ""
}

// paramName returns the name of a parameter, or its position when C declares
// it without one.
func paramName(p *cc.Parameter, i int) string {
	if name := p.Name(); name != "" {
		return name
	}
	return fmt.Sprintf("%d", i+1)
}

// mapConstType maps a parameter type of a C function pointer. C compares two
// function types parameter by parameter, and a pointer to a const type is not
// compatible with a pointer to the same type without const. A So function
// passed as the callback must keep the const, so a pointer to const void maps
// to *c.ConstVoid, and a pointer to a const struct or union maps to a const twin
// type. Every other type maps as usual: a top-level const on a parameter does
// not affect C type compatibility.
func (g *generator) mapConstType(t cc.Type) string {
	pt, ok := t.(*cc.PointerType)
	if !ok {
		return g.mapType(t)
	}
	elem := pt.Elem()
	if !elem.Attributes().IsConst() {
		return g.mapType(t)
	}
	if elem.Kind() == cc.Void {
		return "*c.ConstVoid"
	}

	var twin string
	switch at := elem.(type) {
	case *cc.StructType:
		twin = g.mapConstStructType(at)
	case *cc.UnionType:
		twin = g.mapConstUnionType(at)
	}
	if twin == "" {
		return g.mapType(t)
	}
	return "*" + twin
}

func (g *generator) mapConstStructType(st *cc.StructType) string {
	name := aggregateName(st.Typedef(), st.Tag())
	if name == "" {
		return ""
	}
	// Register the base struct too, so a callback can convert
	// the const value to the base type.
	g.addStruct(name, st)
	return g.addConstStruct(name, st)
}

func (g *generator) mapConstUnionType(ut *cc.UnionType) string {
	name := aggregateName(ut.Typedef(), ut.Tag())
	if name == "" {
		return ""
	}
	g.addUnion(name, ut)
	return g.addConstUnion(name, ut)
}

func (g *generator) mapArrayType(at *cc.ArrayType) string {
	elem := g.mapType(at.Elem())
	if elem == "" {
		return ""
	}
	size := at.Len()
	if size <= 0 {
		return ""
	}
	return fmt.Sprintf("[%d]%s", size, elem)
}

var numeric = map[string]bool{
	"byte": true, "uintptr": true,
	"int8": true, "int16": true, "int32": true, "int64": true,
	"uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true,
}

func zeroValue(typ string) string {
	switch typ {
	case "bool":
		return "false"
	case "string":
		return `""`
	case "any":
		return "nil"
	}
	if numeric[typ] {
		return "0"
	}
	if strings.HasPrefix(typ, "c.") {
		return "0" // every so/c type sobind emits is a number
	}
	if strings.HasPrefix(typ, "*") {
		return "nil"
	}
	if strings.HasPrefix(typ, "func(") {
		return "nil"
	}
	if strings.HasPrefix(typ, "[") {
		return typ + "{}"
	}
	// Struct type - return zero struct.
	return typ + "{}"
}

// aggregateName returns the C name of a struct or union: the typedef name when
// there is one, the tag otherwise. An unnamed type has no C name.
func aggregateName(td *cc.Declarator, tag cc.Token) string {
	if td != nil {
		name := td.Name()
		if name != "" && !strings.HasPrefix(name, "_") {
			return name
		}
	}
	return tag.SrcStr()
}

// typedefName returns the name a type is declared with, or "" if the type is
// not a typedef. For example, "int32_t x" is a cc.Int named "int32_t".
func typedefName(t cc.Type) string {
	td := t.Typedef()
	if td == nil {
		return ""
	}
	return td.Name()
}
