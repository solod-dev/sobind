// Two C names that map to one Solod name once the prefix is stripped.
#define UV_TIMEOUT 5000
extern int uv_timeout;

// A struct tag and a function with the same C name.
struct stat {
	int st_mode;
	long st_size;
};
int stat(const char *path, struct stat *buf);
