// Symbol naming: a header with a uv_ prefix on every C name.

typedef struct uv_loop_s {
	int active_handles;
	int iVersion;
	int type;
} uv_loop_t;

typedef struct uv_buf_s {
	char *base;
	unsigned long len;
} uv_buf_t;

typedef void (*uv_close_cb)(uv_loop_t *loop);

enum uv_run_mode {
	UV_RUN_DEFAULT = 0,
	UV_RUN_ONCE = 1,
	UV_RUN_NOWAIT = 2
};

#define UV_VERSION_MAJOR 1

// A stripped prefix leaves a name that starts with a digit here.
#define UV_3DNOW 1

extern int uv_loop_size;

int uv_run(uv_loop_t *loop, int mode);
int uv_loop_close(uv_loop_t *loop);
int uv_open_v2(const char *path, int flags);
const char *uv_err_name(int err);
