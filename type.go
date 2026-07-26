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
		return g.mapFuncPtrType(t.(*cc.FunctionType))
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
	// A pointer to any char type is a C string, unless it is declared with a
	// fixed-width typedef (uint8_t*), which marks it as a byte buffer.
	isChar := elem.Kind() == cc.Char || elem.Kind() == cc.SChar || elem.Kind() == cc.UChar
	if isChar && fixedWidth[typedefName(elem)] == "" {
		if elem.Attributes().IsConst() {
			return "*c.ConstChar"
		}
		return "*c.Char"
	}
	if elem.Kind() == cc.Function {
		ft, ok := elem.(*cc.FunctionType)
		if ok {
			return g.mapFuncPtrType(ft)
		}
	}
	inner := g.mapType(elem)
	if inner == "" {
		return "any"
	}
	return "*" + inner
}

func (g *generator) mapStructType(st *cc.StructType) string {
	// Use typedef name if available.
	if td := st.Typedef(); td != nil {
		name := td.Name()
		if name != "" && !strings.HasPrefix(name, "_") {
			// Ensure struct is registered.
			g.addStruct(name, st)
			return name
		}
	}
	// Use tag name.
	tag := st.Tag()
	if tag.SrcStr() != "" {
		name := tag.SrcStr()
		g.addStruct(name, st)
		return name
	}
	return ""
}

func (g *generator) mapUnionType(ut *cc.UnionType) string {
	// Use typedef name if available.
	if td := ut.Typedef(); td != nil {
		name := td.Name()
		if name != "" && !strings.HasPrefix(name, "_") {
			// Ensure union is registered.
			g.addUnion(name, ut)
			return name
		}
	}
	// Use tag name.
	tag := ut.Tag()
	if tag.SrcStr() != "" {
		name := tag.SrcStr()
		g.addUnion(name, ut)
		return name
	}
	return ""
}

func (g *generator) mapFuncPtrType(ft *cc.FunctionType) string {
	// Build signature string for deduplication.
	var sig strings.Builder
	sig.WriteString("func(")
	params := ft.Parameters()
	for i, p := range params {
		if i > 0 {
			sig.WriteString(", ")
		}
		sig.WriteString(g.mapType(p.Type()))
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
		sig.WriteString(" ")
		sig.WriteString(g.mapType(rt))
	}
	return sig.String()
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

// typedefName returns the name a type is declared with, or "" if the type is
// not a typedef. For example, "int32_t x" is a cc.Int named "int32_t".
func typedefName(t cc.Type) string {
	td := t.Typedef()
	if td == nil {
		return ""
	}
	return td.Name()
}
