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
sobind [-o output.go] [-pkg name] [-I dir] [-body] [-style c|go] [-strip prefix] [-rename file] <header.h | dir> ...
```

- `-o` - output file (default: stdout)
- `-pkg` - Go package name (default: `main`)
- `-I` - include search directory (repeatable)
- `-body` - emit function bodies (default: declaration only)
- `-style` - symbol naming: `c` keeps the C names (default), `go` emits exported CamelCase names
- `-strip` - C name prefix to remove with `-style=go` (repeatable)
- `-rename` - file of `cname soname` lines that set So names by hand

When given a directory, all `.h` files in it are processed.

## Naming

`-style=c` keeps every C name as it is. Most C names are lower case, so the symbols are unexported and only usable inside the generated package. This is the right mode for a header you bind in the program that uses it.

`-style=go` emits exported CamelCase names, for a binding package other packages import. The C name stays on the `//so:extern` line, so nothing is lost:

```go
//so:extern uv_loop_close
func LoopClose(loop *Loop) c.Int
```

The rules, applied in this order:

1. Remove the first matching `-strip` prefix. The match ignores case and the longest prefix wins.
2. Remove a trailing `_t`: `uv_loop_t` becomes `Loop`.
3. Split on `_` and join the parts in CamelCase: `uv_open_v2` becomes `OpenV2`. A part in upper case only is lowered first, so `UV_RUN_DEFAULT` becomes `RunDefault`. Every other part keeps its inner capitals, so the field `pMethods` becomes `PMethods`.

Go's initialisms do not apply: `uv_tcp_init` becomes `TcpInit`, not `TCPInit`. The mapping stays mechanical, so a name always translates the same way in both directions.

A prefix stays in place when removing it leaves a name that starts with a digit: `SDL_3DNOW` becomes `Sdl3dnow`.

A library often spells one prefix in two cases, `uv_` on functions and `UV_` on macros. One `-strip uv_` covers both. Pass an additional `-strip` if the prefixes differ beyond case:

```
sobind -style=go -strip sqlite3_ -strip SQLITE_ -o sqlite3.go sqlite3.h
```

Two C names that map to one So name are an error, because the So names of one package share a single scope. sobind lists every colliding pair as `cname soname` lines and emits nothing. Pick a different set of prefixes, or paste the lines into a `-rename` file and edit the So names apart.

Some collisions no set of prefixes can fix: libsodium has both `crypto_aead_chacha20poly1305_IETF_ABYTES` and `crypto_aead_chacha20poly1305_ietf_ABYTES`, two symbols that differ only in case. A `-rename` file assigns the final So name to a C name, verbatim, one pair per line:

```
crypto_aead_chacha20poly1305_IETF_ABYTES  AeadChacha20poly1305IetfAbytesLegacy
crypto_aead_chacha20poly1305_ietf_ABYTES  AeadChacha20poly1305IetfAbytes
```

A renamed name skips `-strip` and the CamelCase rules, so it is used as written. Blank lines and lines starting with `#` are ignored. The rename works with either `-style`.

## Example

```
sobind -pkg main -o sqlite3.go sqlite3.h
sobind -I . -o sdl3.go SDL3
sobind -pkg libuv -style=go -strip uv_ -o libuv.go uv.h
```
