typedef void (*on_error)(int code, const char *msg);
typedef int (*compare)(const void *a, const void *b);

void reset(void);

void set_error_handler(on_error handler);

int version(void);

void set_level(int level);

long long checksum(const char *data, unsigned long size);

char *find(const char *name, double weight);

int log_printf(const char *fmt, ...);
