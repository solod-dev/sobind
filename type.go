package main

import (
	"fmt"
	"strings"

	cc "modernc.org/cc/v4"
)

func (g *generator) mapType(t cc.Type) string {
	switch t.Kind() {
	case cc.Void:
		return ""
	case cc.Bool:
		return "bool"
	case cc.Char, cc.SChar, cc.UChar:
		return "c.Char"
	case cc.Short:
		return "int16"
	case cc.UShort:
		return "uint16"
	case cc.Int:
		return "int32"
	case cc.UInt:
		return "uint32"
	case cc.Long, cc.LongLong:
		return "int64"
	case cc.ULong, cc.ULongLong:
		return "uint64"
	case cc.Float:
		return "float32"
	case cc.Double, cc.LongDouble:
		return "float64"
	case cc.Enum:
		return "int32"
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
	if elem.Kind() == cc.Char || elem.Kind() == cc.SChar {
		if elem.Attributes().IsConst() {
			return "*c.ConstChar"
		}
		return "*c.Char"
	}
	if elem.Kind() == cc.UChar {
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

func zeroValue(typ string) string {
	switch typ {
	case "bool":
		return "false"
	case "string":
		return `""`
	case "any":
		return "nil"
	case "c.Char", "byte", "int16", "uint16", "int32", "uint32", "int64", "uint64", "float32", "float64":
		return "0"
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
