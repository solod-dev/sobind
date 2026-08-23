// A header outside the binding. Its declarations only reach the emitter
// through the types the named header refers to.

struct stream {
	int fd;
	char *buf;
};

typedef struct buffer {
	char *base;
	unsigned long len;
} buf_t;

union word {
	int i;
	float f;
};
