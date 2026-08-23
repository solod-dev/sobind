// The header is not allowed by the include rules, so its types stay opaque.
#include "foreign/stream.h"

struct client {
	struct stream *in;
	buf_t *chunk;
};

void dump(struct stream *s);
int fill(buf_t *b, const union word *w);

// The const twin of a type from outside the binding is opaque too.
typedef void (*on_data)(const buf_t *chunk, const union word *w);
