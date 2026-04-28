# sobind

Generates So extern declarations from C header files.

`sobind` parses `.h` files and emits a Go source file with `//so:extern` stubs
for structs, constants, function pointer typedefs, and function declarations.

## Install

```
go install solod.dev/sobind@latest
```

## Usage

```
sobind [-o output.go] [-pkg name] <header.h | dir> ...
```

- `-o` - output file (default: stdout)
- `-pkg` - Go package name (default: `main`)

When given a directory, all `.h` files in it are processed.

## Example

```
sobind -pkg main -o sqlite3.go sqlite3.h
```
