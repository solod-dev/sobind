// Callbacks with const pointer parameters.

typedef struct {
    char *base;
    unsigned long len;
} cb_buf;

typedef struct cb_stat_s {
    long size;
} cb_stat;

union cb_value {
    long i;
    double d;
};

// A pointer to a const struct in a callback keeps the const.
typedef void (*cb_read)(int fd, long nread, const cb_buf *buf);

// A tagged struct and a union behave the same way.
typedef void (*cb_poll)(const cb_stat *prev, const union cb_value *val);

// Two callbacks share one twin type.
typedef void (*cb_recv)(const cb_buf *buf, int flags);

// A const char pointer keeps its own type, and a plain pointer is unchanged.
typedef void (*cb_log)(const char *msg, cb_buf *out);

// A const void pointer keeps the const as well.
typedef int (*cb_cmp)(const void *a, const void *b);

// An array parameter decays to a pointer.
typedef void (*cb_write)(const cb_buf bufs[], unsigned int nbufs);

// A struct field of function pointer type needs the const as much as a typedef.
struct cb_ops {
    void (*on_read)(const cb_buf *buf);
    int version;
};

// A plain function parameter does not need the twin: C accepts a pointer to a
// non-const type where the prototype asks for a pointer to a const type.
void cb_send(const cb_buf *buf, int flags);

void cb_start(int fd, cb_read read_cb);

// An inline function pointer parameter goes through the same mapping.
void cb_each(void (*fn)(const cb_stat *st));
