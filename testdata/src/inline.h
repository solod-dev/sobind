// C functions defined in the header instead of declared.

// An ordinary declaration. The code is in the library.
int decl_only(int x);

// A plain inline definition provides no symbol of its own, so the linked
// library has to carry one. This is the case the `inlined` note reports.
inline int plain_inline(int x) {
	return x + 1;
}

// An extern inline definition provides the symbol itself.
extern inline int extern_inline(int x) {
	return x + 2;
}

// A static inline definition is compiled into every file that includes the
// header, so it needs no symbol from the library.
static inline int static_inline(int x) {
	return x + 3;
}

// A static definition is per file too.
static int static_only(int x) {
	return x + 4;
}
