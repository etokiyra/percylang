# Percy

**A small, statically typed, garbage-collected language that compiles to native
executables. The shipped toolchain emits static binaries directly; your programs
need no C compiler and no runtime.**

<p>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-GPL--3.0--or--later%20%2B%20MIT-ff69b4.svg"></a>
  <img alt="Status" src="https://img.shields.io/badge/status-early%20%C2%B7%20v0.1%20provisional-ffb6c1.svg">
  <img alt="Platform" src="https://img.shields.io/badge/platform-linux%20x86--64-9370db.svg">
  <img alt="Tool" src="https://img.shields.io/badge/tool-purr-ff1493.svg">
  <img alt="Files" src="https://img.shields.io/badge/files-.pcy-90b99f.svg">
</p>

Percy is designed to be easy to learn, cheap to run, and simple to ship. Programs
are checked at compile time, compiled ahead of time to static binaries, and run
without a runtime dependency, an interpreter, or a virtual machine.

The command-line tool is **`purr`**. The language is **Percy** (`percylang` where a
longer identity is needed). Source files use the **`.pcy`** extension.

---

## Quick start

Building the toolchain from a checkout needs Go, a C compiler, and Boehm GC
(`libgc`) for the frozen reference compiler:

```sh
git clone https://github.com/etokiyra/percylang.git
cd percylang

# Build the bootstrap. The repository root has a purr/ source directory,
# so the output goes outside it.
go build -o /tmp/purr-bootstrap ./cmd/purr

# Build the shipped tool and put it on PATH.
/tmp/purr-bootstrap build purr.pcy -o ~/.local/bin/purr
```

Then, with only `purr` on `PATH`:

```sh
purr new hello-world
cd hello-world
purr run
```

`purr new` writes a manifest, a `src/` directory, and a `.gitignore`. `purr run`
builds into a temporary directory and executes the program. The toolchain that
does this is written in Percy and emits a static ELF directly.

A one-line installer is planned for `kiyra.lol`; until then, the source build
above is the supported path.

---

## A taste

```percy
struct Point {
    float x;
    float y;
}

enum Shape {
    Circle,
    Square,
    Triangle
}

fn describe(Shape shape) -> string {
    match (shape) {
        Circle { return "circle"; }
        Square { return "square"; }
        Triangle { return "triangle"; }
    }
}

fn main() {
    int[] values = new int[0];
    for (int i = 1; i <= 5; i++) {
        push(values, i * i);
    }

    int total = 0;
    for (int i = 0; i < length(values); i++) {
        total += values[i];
    }

    Point origin = Point { x = 0.0, y = 0.0 };
    print(describe(Shape.Square), total, origin.x, values[4]);
}
```

```text
$ purr run taste.pcy
square 55 0 25
```

Tagged unions carry typed payloads, and `match` binds them only after the tag is
checked:

```percy
union Expression {
    IntegerLiteral { int value; }
    Add { Expression left; Expression right; }
    Missing
}

fn evaluate(Expression expression) -> int {
    match (expression) {
        IntegerLiteral(value) { return value; }
        Add(left, right) { return evaluate(left) + evaluate(right); }
        Missing { return 0; }
    }
}
```

---

## Language

- **Static and explicit.** Variables and parameters carry explicit types; there is
  no type inference and no implicit conversion.
- **Safe by construction.** No null and no implicit conversions. Array accesses
  are bounds-checked, and a violation fails deterministically instead of
  corrupting memory.
- **Exhaustive matching.** `enum` and tagged `union` values are inspected with
  `match`, which must handle every variant and can bind each field.
- **Memory is managed.** A garbage collector owns the heap; Percy code never frees
  by hand.
- **Familiar control flow.** `if`/`else if`/`else`, `while`, C-style `for`,
  `break`, and `continue`.
- **A small, complete core.** `int`, `float`, `bool`, `string`, `byte`, arrays,
  string-keyed maps, structs, enums, unions, modules, and `import`.
- **Deterministic runtime failures.** A failed bounds check, division by zero, or
  invalid match tag writes a fixed message to standard error and exits nonzero.
- **A recoverable built-in family.** File, directory, environment, and process
  operations return result unions instead of terminating the program.
- **Native output.** The toolchain lowers a checked program to a typed IR, selects
  x86-64, and writes a static ELF64 executable.

The language has no generics, traits, closures, exceptions, or concurrency yet.
`SPEC.md` is the source of truth for what exists and what is deliberately
deferred.

---

## The `purr` toolchain

| Piece | What it does | Status |
| --- | --- | --- |
| `purr new` | scaffold a binary or library project with the standard layout | works |
| `purr build` | compile a source file, or a project in the current directory | works |
| `purr run` | build into a temporary directory, execute, and clean up | works |
| `purr.toml` | the strict `[package]` manifest subset | works |
| `purr add` / `remove` | manage dependencies | planned |
| `purr test` | compile and run a project's tests | planned |
| `purr fmt` | one canonical formatting, no options | planned |
| `purr doc` | generate browsable documentation | planned |
| `purr install` / `publish` | the package ecosystem | planned |
| `purr lsp` | diagnostics, completion, navigation, hover | planned |

With no source argument and a `purr.toml` in the current directory, `build` and
`run` enter project mode: they read the manifest, use `src/main.pcy`, and write
`target/<name>`. Library projects are scaffolded, but building one remains an
explicit deferred error.

---

## Project status

| Area | State |
| --- | --- |
| Language core | implemented: types, control flow, functions, structs, enums, tagged unions, `match`, arrays, maps, strings, bytes, modules, recoverable built-ins |
| Compiler frontend | lexer, parser, checker, loader, and semantic model implemented in both the Go reference and Percy |
| Native backend | default for the shipped `purr`: typed IR, x86-64 selector, static ELF64 writer, conservative mark-sweep GC, exact `%g` float printing, 16-byte System V stack alignment |
| C backend | complete and kept as the frozen reference; the Go CLI still uses it and the bootstrap still needs a C compiler |
| Self-hosting | the C pipeline and the native pipeline each reach a byte-identical fixpoint; Go still builds stage 1 |
| Standard library | not started; the currently available surface is the built-in set |
| Package management, formatter, test runner, docs, LSP | planned |
| Platforms | Linux x86-64 for the native backend; the C reference is portable in principle |

The full test suite is differential: every fixture and corpus input is run through
the Go reference and the Percy implementation, and their generated output,
diagnostics, native ELFs, and program behavior are compared byte for byte.

---

## Repository layout

| Path | Contents |
| --- | --- |
| `SPEC.md` | the language specification and the list of deferred decisions |
| `AGENTS.md` | project process, architecture rules, and the phase history |
| `VISION.md` | what Percy is for and the three kinds of independence it targets |
| `cmd/purr/` | the Go CLI: frozen bootstrap, test harness, and differential reference |
| `lexer/`, `parser/`, `checker/`, `loader/`, `codegen/`, `runtime/` | the Go reference implementation |
| `selfhost/` | the Percy-written lexer, parser, checker, C generator, loader, driver, and native backend |
| `purr/` | the Percy-written `purr` toolchain (`purr.pcy`, `purr/*.pcy`) and the embedded C runtime |
| `runtime/csrc/` | the MIT-licensed C runtime linked into programs built by the C reference |
| `runtime/pcy/` | the MIT-licensed Percy runtime helpers used by the native backend |
| `tests/` | end-to-end fixtures and their expected output |
| `tests/native/` | the native backend's execution corpus |
| `tools/` | `embed_runtime`, which regenerates `purr/embedded.pcy` |

The compiler is GPL-3.0-or-later. The runtime and everything linked into a
compiled program are MIT, so programs you write in Percy are yours to license as
you choose.

---

## Building and testing

The reference compiler and the bootstrap are Go programs:

```sh
go build -o /tmp/purr ./cmd/purr
go vet ./...
go test ./...
```

The suite builds the Percy-written compiler from source and drives it against the
Go reference. It expects `cc`, `gcc`, or `clang` and `libgc` for the reference
pipeline, and it verifies the native pipeline with no C compiler at all.

To regenerate the embedded runtime after changing `runtime/csrc/`:

```sh
/tmp/purr-bootstrap build tools/embed_runtime.pcy -o /tmp/embed_runtime
/tmp/embed_runtime .
```

---

## Contributing

Read `AGENTS.md` first. In short:

1. `SPEC.md` is the source of truth. If code and specification disagree, the
   specification wins until it is deliberately changed.
2. Never change established language semantics without authorization; update
   `SPEC.md` first and call the change out.
3. Every language feature needs tests, including a `.pcy` program with a matching
   `.expected` file where the behavior is observable.
4. Run `go test ./...`, `go vet ./...`, and `gofmt` before finishing.

The repository is in an early, fast-moving state. Small, focused changes with
clear commit messages are the most useful kind of contribution.

---

## License

Percy is licensed under **GPL-3.0-or-later**. The runtime under `runtime/csrc/`
and `runtime/pcy/` is **MIT**, so compiled programs do not inherit the compiler's
license. See `LICENSE`.
