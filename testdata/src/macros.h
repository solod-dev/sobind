#define VERSION "1.2.3"
#define MAX_SIZE 4096
#define ENABLED 1
#define LIMIT (MAX_SIZE - 1)

// Aliases: another macro, with or without parentheses.
#define CAPACITY MAX_SIZE
#define BOUND (LIMIT)
#define TITLE VERSION

// Strings: adjacent literals concatenate.
#define GREETING "hello" " world"
#define QUOTED "say \"hi\""

// Strings sobind can't point at: reported, not guessed.
#define WIDE L"abc"
#define TAIL (VERSION + 1)

// Floats: a single literal, with any suffix or exponent, and signed.
#define SCALE 0.5
#define RATIO 0.5f
#define PRECISE 1.0L
#define EPSILON 1e-9
#define HALF .5
#define WHOLE 5.
#define BYTE_MAX 0x1p8
#define OFFSET -1.5
#define MARGIN (0.25)
#define FACTOR SCALE

// Floats sobind can't compute: reported, not guessed.
#define AREA (0.5 * 2)
#define THIRD (1.0 / 3.0)
#define DOUBLED (SCALE + 1)

// Skipped: no literal value, and a function-like macro.
#define API extern
#define MIN(a, b) ((a) < (b) ? (a) : (b))

// Skipped: cc computes a value for these, but the wrong one.
enum unit { METER = 1, SECOND = 2 };
#define BASE SECOND
#define STEP (SECOND + 1)
#define UMAX ((unsigned)-1)
#define PAIR {1, 2}
