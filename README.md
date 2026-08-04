# sobind

Generates So extern declarations from C header files.

`sobind` parses `.h` files and emits a Go source file with `//so:extern` stubs
for structs, unions, constants, function pointer typedefs, and function declarations.

Note that `sobind` is far from finished and can't handle many situations correctly. It's still useful, but you should treat the generated file as a starting point, not as the final result.

## Install

```
go install solod.dev/sobind@latest
```

## Usage

```
sobind [-o output.go] [-pkg name] [-I dir] [-body] <header.h | dir> ...
```

- `-o` - output file (default: stdout)
- `-pkg` - Go package name (default: `main`)
- `-I` - include search directory (repeatable)
- `-body` - emit function bodies (default: declaration only)

When given a directory, all `.h` files in it are processed.

## Example

```
sobind -pkg main -o sqlite3.go sqlite3.h
sobind -I . -o sdl3.go SDL3
```
