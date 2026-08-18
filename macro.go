package main

import (
	"strconv"
	"strings"

	cc "modernc.org/cc/v4"
)

// macroKind is what an object-like macro expands to.
type macroKind int

const (
	macroOther  macroKind = iota // not a constant
	macroInt                     // integer constant expression
	macroFloat                   // constant expression with a float in it
	macroString                  // string literal
	macroWide                    // wide string literal, L"abc"
)

// constPunct is the set of punctuators a constant expression may contain.
// Any other punctuator (a comma, a brace, a stringize operator) means the
// macro is not a constant, so the value cc computes for it is meaningless.
var constPunct = map[rune]bool{
	'(': true, ')': true, '?': true, ':': true,
	'+': true, '-': true, '*': true, '/': true, '%': true,
	'~': true, '&': true, '|': true, '^': true, '!': true, '<': true, '>': true,
	rune(cc.LSH): true, rune(cc.RSH): true,
	rune(cc.LEQ): true, rune(cc.GEQ): true, rune(cc.EQ): true, rune(cc.NEQ): true,
	rune(cc.ANDAND): true, rune(cc.OROR): true,
}

// macroValue reports what sobind emits for an object-like macro. kind selects
// the declaration: a numeric macro is a So constant and value is the Go
// constant expression, a string macro is a C string variable and value is the
// C text of the literals. note is a message for the reader when the macro
// names a value sobind cannot express. Everything is empty for a macro that is
// not a constant at all (#define API extern).
func macroValue(m *cc.Macro, macros map[string]*cc.Macro) (kind macroKind, value, note string) {
	kind = classifyMacro(m, macros, map[string]bool{})
	switch kind {
	case macroInt:
		return kind, formatValue(m.Value()), ""
	case macroFloat:
		// cc evaluates a macro with the preprocessor's integer arithmetic,
		// which turns every float into 0, so the source text is the only
		// source of truth - and it only covers a single literal.
		if text := floatText(m, macros); text != "" {
			return kind, text, ""
		}
		return kind, "", "float expression, define the constant manually"
	case macroString:
		if text := stringText(m, macros); text != "" {
			return kind, text, ""
		}
		return kind, "", "string expression, define the constant manually"
	case macroWide:
		// so/c has no wchar_t type to point at.
		return kind, "", "wide string, define the constant manually"
	}
	return macroOther, "", ""
}

// classifyMacro reports what a macro expands to, following references to other
// macros. A macro is a constant only if every token of the full expansion is a
// literal, a constant expression punctuator, or another object-like macro.
func classifyMacro(m *cc.Macro, macros map[string]*cc.Macro, seen map[string]bool) macroKind {
	name := m.Name.SrcStr()
	if seen[name] {
		return macroOther // cyclic reference
	}
	seen[name] = true
	defer delete(seen, name)

	toks := m.ReplacementList()
	if len(toks) == 0 {
		return macroOther
	}

	hasFloat, hasString, hasWide := false, false, false
	for _, tok := range toks {
		switch tokenKind(tok, macros, seen) {
		case macroOther:
			return macroOther
		case macroFloat:
			hasFloat = true
		case macroString:
			hasString = true
		case macroWide:
			hasWide = true
		}
	}

	switch {
	case hasFloat && (hasString || hasWide):
		return macroOther
	case hasFloat:
		return macroFloat
	case hasWide:
		// C concatenates a narrow literal next to a wide one into a wide one.
		return macroWide
	case hasString:
		return macroString
	}
	return macroInt
}

// tokenKind returns the kind a single token contributes to its macro, or
// macroOther if the token means the macro is not a constant. A punctuator
// contributes macroInt: it leaves the kind to the literals around it.
func tokenKind(tok cc.Token, macros map[string]*cc.Macro, seen map[string]bool) macroKind {
	switch rune(tok.Ch) {
	case rune(cc.IDENTIFIER):
		// A name that is not an object-like macro is a keyword, a cast, or an
		// enum constant. cc computes a value for those too, but a wrong one.
		ref := macros[tok.SrcStr()]
		if ref == nil || ref.IsFnLike {
			return macroOther
		}
		return classifyMacro(ref, macros, seen)
	case rune(cc.PPNUMBER), rune(cc.INTCONST), rune(cc.FLOATCONST):
		if isFloatLit(tok.SrcStr()) {
			return macroFloat
		}
		return macroInt
	case rune(cc.CHARCONST), rune(cc.LONGCHARCONST):
		return macroInt
	case rune(cc.STRINGLITERAL):
		return macroString
	case rune(cc.LONGSTRINGLITERAL):
		return macroWide
	}

	if constPunct[rune(tok.Ch)] {
		return macroInt
	}
	return macroOther
}

// isFloatLit reports whether a preprocessing number is a float constant.
// A hex number needs a dot or a binary exponent, since 0x1e5 is an integer;
// a decimal number needs a dot or a decimal exponent.
func isFloatLit(s string) bool {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return strings.ContainsAny(s, ".pP")
	}
	return strings.ContainsAny(s, ".eE")
}

// floatText returns the Go literal for a macro that expands to a single float
// constant, optionally signed, or "" for anything more complex.
func floatText(m *cc.Macro, macros map[string]*cc.Macro) string {
	toks := resolveMacro(m, macros)

	sign := ""
	if len(toks) == 2 && (rune(toks[0].Ch) == '+' || rune(toks[0].Ch) == '-') {
		sign = toks[0].SrcStr()
		toks = toks[1:]
	}
	if len(toks) != 1 {
		return ""
	}

	// C has a float suffix (0.5f, 1.0L), So infers the type instead.
	lit := toks[0].SrcStr()
	if n := len(lit); n > 0 && strings.ContainsRune("fFlL", rune(lit[n-1])) {
		lit = lit[:n-1]
	}
	// Rejects what C accepts and So does not, such as digit separators.
	if _, err := strconv.ParseFloat(lit, 64); err != nil {
		return ""
	}
	return sign + lit
}

// stringText returns the C text of a macro that expands to string literals,
// or "" for anything else. C concatenates adjacent literals, so the text keeps
// all of them. macroString only means the expansion has a literal somewhere in
// it, and the operators around the literal change the type: "abc" + 1 is still
// a char*, but "abc" - "def" is an integer. Telling those apart takes a real
// expression evaluator, so anything but literals is rejected.
func stringText(m *cc.Macro, macros map[string]*cc.Macro) string {
	toks := resolveMacro(m, macros)
	if len(toks) == 0 {
		return ""
	}
	parts := make([]string, len(toks))
	for i, tok := range toks {
		if rune(tok.Ch) != rune(cc.STRINGLITERAL) {
			return ""
		}
		parts[i] = tok.SrcStr()
	}
	return strings.Join(parts, " ")
}

// resolveMacro returns the replacement list that carries the value of a macro,
// following aliases (#define B A) and dropping redundant parentheses.
func resolveMacro(m *cc.Macro, macros map[string]*cc.Macro) []cc.Token {
	seen := map[string]bool{}
	toks := m.ReplacementList()
	for {
		toks = unparen(toks)
		if len(toks) != 1 || rune(toks[0].Ch) != rune(cc.IDENTIFIER) {
			return toks
		}
		name := toks[0].SrcStr()
		ref := macros[name]
		if ref == nil || seen[name] {
			return toks
		}
		seen[name] = true
		toks = ref.ReplacementList()
	}
}

// unparen drops the parentheses that wrap the whole token list.
func unparen(toks []cc.Token) []cc.Token {
	for len(toks) >= 2 && rune(toks[0].Ch) == '(' && rune(toks[len(toks)-1].Ch) == ')' {
		depth := 0
		for _, tok := range toks[:len(toks)-1] {
			switch rune(tok.Ch) {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == 0 {
				return toks // the first parenthesis closes before the end
			}
		}
		toks = toks[1 : len(toks)-1]
	}
	return toks
}
