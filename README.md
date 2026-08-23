# sobind

Generates So extern declarations from C header files.

`sobind` parses `.h` files and emits a Go source file with `//so:extern` stubs
for structs, unions, constants, variables, function pointer typedefs, and function declarations.

Note that `sobind` is far from finished and can't handle many situations correctly. It's still useful, but you should treat the generated file as a starting point, not as the final result.

## Install

```
go install solod.dev/sobind@latest
```

## Usage

```
sobind [-o output.go] [-pkg name] [-I dir] [-scope dir] [-body] [-style c|go] [-strip prefix] [-rename file] <header.h | dir> ...

Usage:
  -I value
    	include search directory (repeatable)
  -body
    	emit function bodies (default: declaration only)
  -o string
    	output file (default: stdout)
  -pkg string
    	Go package name (default "main")
  -rename string
    	file of 'cname soname' lines that set So names by hand
  -scope value
    	directory whose headers are emitted, beyond the named files (repeatable)
  -strip value
    	C name prefix to remove (repeatable)
  -style string
    	symbol naming: c (keep C names), cap (capitalized), or go (exported CamelCase)
```

When given a directory, all `.h` files in it are processed.

By default only the named headers are emitted; anything they include is parsed but skipped, so system headers stay out of the binding. An umbrella header like `sodium.h` holds no declarations of its own, just includes of the library's `sodium/*.h` headers, so on its own it emits nothing. Point `-scope` at the library's header tree to emit every header under it:

```
sobind -o extern.go -scope libsodium/include libsodium/include/sodium.h
```

## Naming

### Original C names

`-style=c` keeps every C name as it is. Most C names are lower case, so the symbols are unexported and only usable inside the generated package.

### Capitalized names

`-style=cap` capitalizes every C name for export without otherwise changing it. It's the simplest way to export the binded API from your package.

Combine it with `-strip=<prefix>` to remove the C name prefix:

- `uv_buf_init` → `Buf_init`
- `sqlite3_busy_timeout` → `Busy_timeout`

### Go-like names

`-style=go` emits exported CamelCase names.

The rules:

1. Remove a trailing `_t`: `loop_t` becomes `Loop`.
2. Split on `_` and join the parts in CamelCase: `open_v2` becomes `OpenV2`. A part in upper case only is lowered first, so `RUN_DEFAULT` becomes `RunDefault`. Every other part keeps its inner capitals, so the field `pMethods` becomes `PMethods`.

Combine `-style=go` with `-strip=<prefix>` to remove the C name prefix:

- `uv_buf_init` → `BufInit`
- `sqlite3_busy_timeout` → `BusyTimeout`

### Renaming

Using name-modifying flags like `-style` and `-strip` can lead to name collisions.

For example, `libuv.h` defines a `UV_FILE` macro. With `-strip=uv_` it maps to `FILE` and collides with the `FILE` typedef from `stdio.h`.

To solve such collisions, use the `-rename=<filename>` flag. The file maps a C name to a So name, one pair per line:

```
# The symbol will be emitted as UvFile.
UV_FILE  UvFile
```

A line with a C name without a So name drops the symbol instead:

```
# The symbol won't be emitted at all.
Fts5Tokenizer
```

## Example

```
sobind -o sqlite3.go -pkg main sqlite3.h
sobind -o sdl3.go -I . SDL3
sobind -o libuv.go -pkg libuv -style=go -strip uv_ uv.h
```
