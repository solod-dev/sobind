#define VERSION "1.2.3"
#define MAX_SIZE 4096
#define ENABLED 1
#define LIMIT (MAX_SIZE - 1)

// Skipped: no literal value, and a function-like macro.
#define API extern
#define MIN(a, b) ((a) < (b) ? (a) : (b))
