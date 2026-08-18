// The fixed-width typedefs are declared here instead of included from
// <stdint.h> to keep the test independent of system headers.
typedef signed char int8_t;
typedef short int16_t;
typedef int int32_t;
typedef long long int64_t;
typedef unsigned char uint8_t;
typedef unsigned short uint16_t;
typedef unsigned int uint32_t;
typedef unsigned long long uint64_t;
typedef unsigned long uintptr_t;
typedef unsigned long size_t;
typedef long ssize_t;
typedef long ptrdiff_t;
typedef long intptr_t;

struct Scalars {
    _Bool flag;
    char ch;
    signed char sch;
    unsigned char uch;
    short sh;
    unsigned short ush;
    int i;
    unsigned int ui;
    long l;
    unsigned long ul;
    long long ll;
    unsigned long long ull;
    float f;
    double d;
};

struct Sized {
    int8_t i8;
    int16_t i16;
    int32_t i32;
    int64_t i64;
    uint8_t u8;
    uint16_t u16;
    uint32_t u32;
    uint64_t u64;
    uintptr_t addr;
};

// The width of these types follows the target, so they keep their C name.
struct Target {
    size_t size;
    ssize_t nread;
    ptrdiff_t delta;
    intptr_t addr;
    long double ld;
};

enum Mode {
    MODE_OFF,
    MODE_ON
};

struct Buf {
    uint8_t *data;
    uint64_t size;
    char *name;
    const char *label;
    void *user;
    int32_t nums[4];
};

int process(struct Buf *buf, enum Mode mode, size_t n, const char *msg);
ssize_t read_buf(struct Buf *buf, void *dst, size_t n);

int generichash(unsigned char *out, size_t outlen,
                const unsigned char *in, unsigned long long inlen,
                const unsigned char *key, size_t keylen);
