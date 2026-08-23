// C declarations sobind cannot map exactly. Each one gets a `// sobind:` note.

// A callback is called through this exact signature, so an unmappable
// parameter cannot fall back to the any type the way a plain function does.
typedef double _Complex (*transform)(double _Complex z, int n);

// A bitfield has no So equivalent, so the whole type is opaque.
struct flags {
	unsigned int visible : 1;
	unsigned int width : 15;
};

// An anonymous member has no name to map.
struct packet {
	int kind;
	struct {
		int lo;
		int hi;
	};
};

// A named member of an anonymous type has nothing to name the type by.
struct sample {
	int id;
	struct {
		double x;
		double y;
	} point;
};

// A member of a type sobind does not know stays unmapped.
struct wave {
	int freq;
	double _Complex amp;
};

// A flexible array member has no size.
struct blob {
	int len;
	char data[];
};

// An incomplete array has no size either.
extern const char build_id[];

// _Complex has no So equivalent, in a parameter or in a result.
double _Complex scale(double _Complex z, int n);
