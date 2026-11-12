# AGENTS.md

Project instructions for everyone (human or AI) working on Percy.

## Current checkpoint (2026-11-12)

**State:** the language is **Percy**, the tool is **`purr`**, and source files
are **`.pcy`**. The repository is `etokiyra/percylang`; the local checkout is
`~/Projects/percy`. `main` is clean and level with `origin/main`.

**Green:** `go test ./... -count=1 -timeout 60m` passes in full
(`percy/checker`, `percy/cmd/purr`, `percy/codegen`, `percy/lexer`,
`percy/loader`, `percy/parser`, `percy/runtime`), `go vet ./...` is clean, and
`gofmt -l .` prints nothing.

**Last landed:**
- Phase 10.39: `break`/`continue` in the lexers, parsers, checkers, both C
  generators, and the native lowering; `match` emits an `if`/`else if` chain so
  a `break` inside an arm targets the enclosing loop.
- Phase 10.40: `push(array, value)`, the array capacity word, capacity doubling,
  and the C and native runtime paths, with fixtures and corpora.
- Phase 10.41: the full rename to Percy — `.pcy` extension,
  `pcy_*` identifiers, `percy_runtime.{h,c}`, the `percy` Go module,
  `runtime/pcy`, and updated README, VISION, site, and profile.

**Next, in order:**
1. Standard library, first slice: `std/string` (`starts_with`, `ends_with`,
   `contains`, `index_of`, `split`, `join`, `replace`, `trim`, `to_upper`,
   `to_lower`, `repeat`, `int_to_string`, `string_to_int`) in Percy over the
   existing built-ins, plus the loader rule for a standard-library root.
2. Language conveniences: `for (value in array)`, array literals, map iteration
   and `remove`, and array slices.
3. Tooling: `purr test`, `purr fmt`, then dependencies, install, and the
   registry.
4. Performance: hash maps in the native runtime, full register allocation,
   inlining and escape analysis, bounds-check elimination.
5. Independence: a Go-free bootstrap artifact and the one-line installer.
6. Platforms and ecosystem: arm64/macOS, LSP, docs generation, publishing.

**Known limits:** no standard library; native output is Linux x86-64 only; the
native map is a linear packed table; Go still builds stage 1; library object
builds are an approved deferred error; the Go CLI does not dispatch `new` yet.

---

## Source of truth

1. `SPEC.md` is the source of truth for language semantics. If code and
   `SPEC.md` disagree, the specification wins until it is deliberately changed.
2. Never change established language semantics without explicit authorization.
   If a change is needed, update `SPEC.md` first and call the change out.
3. If a design decision is unresolved and affects implementation, ask before
   proceeding when necessary. Do not silently invent semantics. Record the open
   question under "Deferred decisions and unresolved semantics" in `SPEC.md`.

## Testing

4. Every language feature must have appropriate tests. Once executable language
   features exist, add a `.pcy` program and a matching `.expected` file under
   `tests/`, and compare the program's actual output against the expected output.
5. Run `go test ./...` before completing a task. Also run `go vet ./...` and
   `gofmt` (or `gofmt -l .`) when Go code changed. Build the CLI with
   `go build -o /tmp/purr ./cmd/purr` when the command or its packaged code
   changed. The repository-root `purr/` source directory prevents the default
   output name `purr`.
6. Never claim a feature is implemented without verifying it. Run the relevant
   tests or commands and report what actually happened.

Verification runs in tiers, so correctness claims stay tied to what a run
actually covered: the required baseline above is the full suite under the
discovered C compiler; the `CC=gcc` and `CC=clang` runs the README documents
belong to milestone and freeze runs (for example the 9.9 freeze report),
where every claim is checked under each compiler; ordinary slices verify the
full suite under one compiler and name anything they could not run. The
test harness builds the same large binaries repeatedly — the stage-1 driver,
both purr-pcy builds, and the stage probes — and caches them under
`$HOME/.cache/percy-verify/builds`, keyed by every input's content, the
resolved compiler, and the GC flags, with `PERCY_TEST_CACHE` overriding or
disabling the directory. A cache hit can never be stale: any change to the
sources a build depends on produces a different key, and every cache key's
verification happens when its bytes are first built.

The `.pcy` files under `tests/` are end-to-end tests: the lexer tokenizes them,
the parser builds a syntax tree, the checker validates them, the code generator
emits C, and the native executable's output is compared byte-for-byte with the
matching `.expected` file. Some fixtures are multi-file: `tests/modules/` has a
root file with imports; add the matching `.expected` next to the root file. Keep
the `.expected` files in sync with actual, verified program output; never edit
them to match buggy behavior.

## Architecture

7. Keep lexer, parser, loader, checker, code generator, and runtime concerns
   separated into their own packages: `lexer`, `parser`, `loader`, `checker`,
   `codegen`, `runtime`. A package must not reach into another phase's
   responsibilities.
8. Keep the compiler's architecture easy to test and extend. Prefer clear
   interfaces and table-driven tests over clever or tightly coupled code.
9. Use descriptive identifiers. Avoid unexplained single-letter variable names.
   Conventional short names are fine for loop indices and local coordinates.
10. Components written in Percy for the eventual self-hosting compiler live
    under `selfhost/`. They are compiled and tested by the bootstrap compiler but
    are not wired into `purr build` or `purr run`, which keep using the Go
    implementation. Do not make the production pipeline depend on them without
    explicit authorization.

## Process

11. Make small, focused changes. One coherent change per commit, with a clear
    message.
12. Do not begin future phases without explicit authorization. Phase 7.0
    authorizes the **design** of Goal 2 (a native backend and a runtime with no
    C), but not its implementation; implementation needs a separate, later
    authorization after the design is approved. Do not implement self-hosting
    beyond the proven C-level fixpoint, a native backend, or standard-library
    features without explicit authorization.
13. Update documentation when established behavior changes: `SPEC.md` for
    language behavior, `AGENTS.md` for process and architecture.

## Licensing

Percy is GPL-3.0-or-later, with one deliberate exception: the runtime under
`runtime/csrc/` is MIT. The split exists to serve two goals at once. The GPL
keeps the compiler free and stops anyone from forking this implementation into a
closed product. The permissive runtime keeps compiled programs free of the
compiler's license, so people can distribute what they build under any terms
they choose.

14. Keep the toolchain GPL-3.0-or-later and code linked into compiled programs
    MIT. Do not move runtime or standard-library sources under the GPL, and do
    not apply the GPL to anything that ends up inside a user's executable.
15. When adding a component, decide which side of that line it is on and give it
    the matching notice. The compiler and its tools are GPL; anything compiled
    and linked into user programs is MIT.
16. Leave the license notices at the top of the `runtime/csrc/` files intact.
    Those files are copied next to generated code and compiled by third parties,
    so the notice has to travel with them.

## Naming

The language is **Percy** (the project is **Percy**, `percylang` where a longer
identity is needed), source files are **`.pcy`**, and the command-line tool is
**`purr`**. `purr` is the tool's own name and stays independent of the language
name: the language is **Percy** while the tool is **`purr`**. The project
manifest is `purr.toml` and the lockfile is `purr.lock`.

17. Write documentation against `purr`. The compiler binary is built from
    `cmd/purr` and installed as `purr`. The language is **Percy** and source
    files are `.pcy`; generated C, runtime files, and identifiers use the `pcy_`
    and `pcy_rt_*` prefixes, and generated C includes `percy_runtime.h`.

## Current phase

Phase 1 delivered the project skeleton, the provisional specification, the
example programs, and the lexer. Phase 2A delivered the parser and the abstract
syntax tree. Phase 2B delivered semantic analysis and the type checker. Phase 2C
delivered the C code generator, the C runtime, and native compilation through
`purr build` and `purr run`. Phase 3A delivered dynamic strings:
concatenation with `+` and `+=`, and content-based equality, over the existing
NUL-terminated representation. Phase 3B delivered dynamic arrays: typed array
declarations, `new Element[length]` creation, bounds-checked indexing,
`length`, and GC-managed reference semantics. Phase 3C delivered file I/O
(`read_file`), a module system with `import`, cross-module visibility, and
multi-file compilation. Phase 3D delivered enums and tagged unions: named
alternatives, variants with typed payloads, safe construction, and an exhaustive
`match` statement for inspecting variants. Phase 3E delivered typed maps:
`map[string]V` with string keys, GC-managed reference semantics, and a hash
table in the runtime. Phase 3F prepared for a Percy-written lexer: string byte
length, bounds-checked byte indexing, and byte ranges (`substring`). Phase 3G
delivered the first real component written in Percy: a lexer under
`selfhost/lexer` that mirrors the Go reference lexer and is checked against it by
a differential corpus of 772 inputs in `cmd/purr`. Phase 3H gave strings a
stored byte length alongside the bytes, so `length` and indexing are
constant-time while the bytes stay NUL-terminated; the observable string
semantics did not change, and the Percy-written lexer now scans linearly. Phase
4A added the next self-hosted component: a parser under `selfhost/parser`, driven
by `selfhost/parse.pcy`, that reproduces the Go reference parser's grammar,
precedence, tree, and diagnostics, and is checked against it by a differential
corpus of 262 inputs in `cmd/purr`. Phase 4B added a semantic checker under
`selfhost/checker`, driven by `selfhost/check.pcy`, that reproduces the Go
checker's passes, rules, and diagnostics. Phase 4C exposed the checker's resolved
semantic model — the types, symbols, variants, and declarations a generator reads
— for a future code generator. Phase 4D.1 began the C code generator written in
Percy: its C naming, C type mapping, string-literal escaping, and bounded output
accumulation under `selfhost/codegen`, exposed by `selfhost/codegen.pcy` and
checked against the Go generator in `cmd/purr`. Phase 4D.2 added the declaration
emitter under `selfhost/codegen/declarations.pcy`, driven by
`selfhost/declarations.pcy`: it reproduces the nine sections the reference
generator emits before any function body, byte for byte, and is checked against
the reference over a targeted corpus, the checker corpus, and representative
native compilations. Phase 4D.3 added the core expression emitter under
`selfhost/codegen/expressions.pcy`, driven by `selfhost/expressions.pcy`: integer,
floating-point, boolean, and string literals, identifiers, unary expressions, and
binary expressions, checked against the reference node by node. Phase 4D.4 added
the string literal pool under `selfhost/codegen/literals.pcy`, which the expression
emitter fills as it renders and which the differential harness compares with the
reference's own pooled declarations. Phase 4D.5 added the statement and function
emitter under `selfhost/codegen/statements.pcy`, driven by `selfhost/functions.pcy`:
blocks, local declarations, expression statements, `if`/`else`, `while`, `for`,
`return`, and `match`, checked against the reference's own function definitions byte
for byte. At that point assignments, calls, member access, struct and variant
literals, allocation, and indexing were still reported as not implemented rather than
rendered when a statement needed one. Phase 4D.6 added the translation unit emitter
under `selfhost/codegen/unit.pcy`, driven by `selfhost/unit.pcy`: the entry point rule,
the C entry point, and the assembly of the declaration sections, the literal pool, the
bodies, and the entry point into one complete translation unit, checked against the
reference's whole output byte for byte. Phase 4D.7 added calls: user-defined
functions, cross-module calls, and the built-ins `print`, `length`, `has`,
`substring`, and `read_file`, so a generated program can now produce output, which
the differential harness compiles, runs under GCC and Clang, and compares against the
reference program's output. Phase 4D.8 added member access and qualified variant
references, struct and variant literals, array and map creation, and indexing under
`selfhost/codegen/expressions.pcy`, so a program that constructs and inspects values
is now emitted byte for byte. Phase 4D.9 added the assignment expression, the last
unimplemented expression kind: plain assignment to every l-value form, map-entry
insertion, compound `+=` and `-=`, `++`/`--`, and string append, with the string and
map runtime helpers named in `names.pcy`. Every expression kind is now emitted, all
nine top-level fixtures under `tests/` compile, run, and match their expected output
through the Percy backend under GCC and Clang. Phase 4E.1 added the module loader
under `selfhost/loader`, driven by `selfhost/load.pcy` and
`selfhost/loadunit.pcy`: it loads an absolute root path into the same ordered module
set as the Go loader, with the same canonical paths, ids, depth-first order, cycle
and duplicate handling, outside-root diagnostic, and lex/parse diagnostics. All
15 multi-file checker corpus programs, 33 targeted trees, and `tests/modules/` match
the reference loader byte for byte, and `tests/modules/` runs end-to-end through the
fully Percy-written loader, lexer, parser, checker, and generator. Missing and
unreadable files were completed by 5.5; relative root paths remain out of scope
because the self-hosted runtime has no working-directory primitive. Phase
5.1 added the `argv()` built-in end to end in both pipelines: the generated C entry
point now takes `argc`/`argv`, saves them with `pcy_rt_set_args`, and user code sees
the vector through `pcy_rt_argv`. The phase also added a runtime test with zero, one,
and several arguments, including an empty argument and one containing spaces. Phase
5.2 added `eprint(...)` end to end: the runtime's `pcy_rt_eprint_*` helpers mirror
the print helpers, the newline helper flushes stdout before the diagnostic and then
flushes stderr, and the runtime fixture captures stdout and stderr separately through
both pipelines. Phase 5.3 added `exit(status)` and one shared flush path:
`pcy_rt_exit` flushes stdout then stderr and terminates, `pcy_rt_fail` routes through
it while keeping its message and status 1, and the runtime fixture checks stdout,
stderr, and the exact status, including POSIX low-eight-bit behavior. Phase 5.6
added the Percy-written driver `selfhost/driver.pcy` and the temporary POSIX-sh
wrapper `selfhost/driver/purr-bootstrap.sh`: the driver takes one source path,
emits byte-identical C on stdout, reports diagnostics on stderr, and uses exit
statuses 0/1/2; the wrapper builds and runs fixtures and `tests/modules/` under
GCC and Clang. The implementation order changed: `ReadFileResult`/`try_read_file`
(5.4) and loader read-error parity (5.5) were postponed until after the fixpoint
(5.7). Both are now complete: `try_read_file` is implemented in both pipelines,
and the loader reports missing, unreadable, and directory paths with the
reference's `cannot read source file: ` diagnostic. Phase 5.7 reached the C-level fixpoint: Go builds
stage 1 from `selfhost/driver.pcy`, stage 1 builds stage 2, stage 2 builds stage 3,
and the generated C for `driver.pcy` is byte-identical across Go and all three
stages; stage 2 also reproduces the Go reference for all nine fixtures and
`tests/modules/`. The stage binaries were byte-identical in the measured run, but
the claim rests on the C, not the binaries.

Phase 6.0 renamed the command-line tool from its original name to `purr`: the source
directory is `cmd/purr`, the built binary is `purr`, CLI text says `purr`, the
temporary wrapper is `selfhost/driver/purr-bootstrap.sh` with
`PURR_BOOTSTRAP_DRIVER` and `PURR_RUNTIME_DIR`, and tests, build commands, and
documentation use the new name. The language name, source extension, and
generated identifiers were unchanged by that phase; Phase 10.41 renamed the
language to Percy, the extension to `.pcy`, and the identifiers to `pcy_`.

Phase 5.4b implemented the adopted `ReadFileResult`/`try_read_file` design in the
C runtime and the Go pipeline: the predeclared union and on-use prelude, the Go
checker/generator, and Go-compatible errno text. Phase 5.4c mirrored it in the
Percy-written checker, semantic model, and generator: the checker appends the
fixed prelude as module `n` on a syntactic trigger, reserves `ReadFileResult`,
and registers the builtin after collecting the type; the generator emits the
normal union and a non-static helper and lowers the call to it. The shared
checker, semantic, expression, function, unit, declarations, and loader corpora
grew, and a both-pipeline runtime fixture covers existing, empty, missing,
directory, and unreadable paths under both compilers. Phase 5.5 completed loader
read-error parity: the Percy loader uses `try_read_file`, returns the canonical
path with no position and the `cannot read source file: ` prefix, keeps the
outside-root check before the read, and matches the Go loader byte for byte,
including through stage 1 and stage 2. Phase 8.1 implemented the adopted
defined-integer-arithmetic design in both pipelines and the C runtime:
`+ - *`, unary `-`, `+=`, `-=`, `++`, and `--` wrap modulo 2^64; `/` and `%`
have defined truncation/remainder rules; zero divisors fail with
`integer division by zero`; `INT64_MIN / -1` wraps and `% -1` is zero; decimal
literals above `INT64_MAX` are rejected with `integer literal out of range`;
and generated C calls the MIT `pcy_rt_i64_*` helpers rather than evaluating
signed overflow. Phase 8.2 added `& | ^ ~ << >>`, C#-style precedence and
arithmetic shifts, checked shift counts, and lowercase `0x`/`0b` literals with
64-bit bit-pattern semantics and identical lexical/checker errors. Phase 8.3a
renamed every `byte` identifier in Percy source, fixtures, and embedded test
source to `octet`, with no language change. Phase 8.3b added `byte` as a reserved
unsigned 8-bit type, `byte[]`, byte comparisons and printing, and the four
conversions `int_from_byte`, `byte_from_int`, `string_to_bytes`, and
`bytes_to_string`. Phase 8.4 added the recoverable file and directory built-ins,
their `FileResult` and `ListDirectoryResult` prelude unions, Go-compatible error
text, and the generalized on-use prelude. Phase 8.5 added the recoverable
`try_run` process-launch built-in and its `RunResult` prelude union, with direct
`execve` semantics, inherited environment/working directory/descriptors, status
and signal reporting, and pre-fork stream flushing. Phase 8.6 added the
recoverable `try_current_directory` built-in and its `CurrentDirectoryResult`
prelude union, using the kernel's `getcwd` path, ignoring `$PWD`, and growing its
buffer on `ERANGE`. Phase 8.7 added the compiler-reserved `<runtime>` synthetic
module and its harness-only injection hook, runtime-only intrinsics, C lowering,
and the MIT `runtime/pcy/memory.pcy` helper source. The primitive checklist is
complete. Phase 9.0 produced the design-only `proposals/purr-in-percy.md` for
rewriting `purr`, `purr new`, and `purr.toml`; it is **ADOPTED as the plan;
implementation complete through 9.9**. Phase 9.1 implemented its first step, the `try_getenv`
built-in and predeclared `EnvResult` union, and Phase 9.2 implemented
`try_run_capture` and `CapturedRunResult`, Phase 9.3 implemented
`try_file_info` and `FileInfoResult`, Phase 9.4 implemented
`try_create_temp_directory` and `TempDirectoryResult`, Phase 9.5 embedded the C
runtime sources in `purr/embedded.pcy`, Phase 9.6 implemented the `purr.toml`
subset parser in `purr/manifest.pcy`, Phase 9.7 implemented the `purr new`
module and probe in `purr/new.pcy` and `purr/new_probe.pcy`, and Phase 9.8a
implemented the `purr-pcy` entry point, help/usage, and `new` dispatch in
`purr.pcy` and `purr/cli.pcy`, in both pipelines, including the dedicated
`purr new -h`/`--help` text. Phase 9.8b added the
in-process `build` command in `purr/build.pcy`, and Phase 9.8c implemented the
`run` command in `purr/cli.pcy` with focused both-pipeline tests. Project mode
then added `purr/project.pcy`, current-directory manifest discovery, and
`target/<name>` build/run for binary projects; library builds remain the
approved deferred error. The 9.8c parity suite is now complete: the cases from
`proposals/purr-in-percy.md` Section 12 are covered across
`cmd/purr/purr_build_test.go`, `purr_build_fake_test.go`, `purr_run_test.go`,
`purr_project_test.go`, and `purr_pcy_test.go`, including the last path-only
loader failure (unreadable source), loader import cycles and generated-C
identity, the `.pcy`/`a.out` default-output fallback, lex-error diagnostics,
`pkg-config` absent and failing modes, and `CC`/`cc`/`gcc`/`clang` discovery.
Phase 9.9 then completed the freeze and name swap: the suite's freeze run
passed under the discovered `cc`, `CC=gcc`, and `CC=clang`;
`TestPercyPurrReleaseBuild` verifies the documented release flow from a clean
git checkout (bootstrap `go build -o ./cmd/purr`, then build `purr.pcy`, install
the result as `purr`, and use it for `new` and a project `run`); the Go CLI is
frozen as the bootstrap and testing reference, and the Percy-written sidecar
takes over the `purr` name — `purr-pcy` survives only as the harness label for
pipeline-specific test builds. Library object builds (Proposal Section 12.3)
remain the approved deferred case.
`try_find_program` is
intentionally dropped: PATH search will be Percy code over `try_getenv` and
`try_file_info`.
Phase 9.10 then resolved the deferred relative loader roots:
`selfhost/loader/resolve_root` resolves an absolute and a relative root exactly
as the reference's `canonicalPath` does with `filepath.Abs`, using
`try_current_directory` (the kernel's `getcwd`, ignoring `$PWD`) for the
relative case; `load_program`, the `load.pcy`/`loadunit.pcy` probes, and
`selfhost/driver.pcy` accept relative roots, and the differential corpus grew to
165 relative spellings plus `tests/modules/` and the whole unit pipeline in both
root forms. File writing now unblocks `purr new` and process launch
makes `purr run`-style tooling writable in Percy, both as separate later
phases. A SPEC cleanup of Go/C-flavored wording remains a later candidate. The
native backend and a standard library remain unauthorized.

### Generated runtime embedding

`purr/embedded.pcy` is generated from `runtime/csrc/percy_runtime.h` and
`runtime/csrc/percy_runtime.c` by `tools/embed_runtime.pcy`. Regenerate it from
the repository root with:

```sh
purr build tools/embed_runtime.pcy -o /tmp/embed_runtime
/tmp/embed_runtime .
```

Any change under `runtime/csrc/` requires regenerating and committing
`purr/embedded.pcy`; the three-way drift test regenerates it through the Go
pipeline, the Percy pipeline, and an independent Go reference and fails if the
committed file differs. The embedded strings are MIT-licensed runtime sources
with their notices preserved; the generated file's own header records the
toolchain license.

Because `purr/` is now a source directory, `go build ./cmd/purr` cannot write a
repository-root binary named `purr`; build it to an external path or install
location, for example `go build -o /tmp/purr ./cmd/purr`.

### Manifest parser

`purr/manifest.pcy` is the plain-Percy parser for the strict `[package]`
manifest subset. It has no imports and performs no file I/O; its public API is
`parse_manifest(text) -> ManifestResult`. `purr/manifest_probe.pcy` is the
test/CLI-shaped probe; it lives under `purr/` because the loader only permits
imports below the root source's directory, so `tools/manifest_probe.pcy` could
not import `../purr/manifest.pcy`.

The pinned, randomized, both-pipeline, performance, and sanitizer tests live in
`cmd/purr/manifest_test.go`; the independent Go oracle is in
`cmd/purr/manifest_oracle_test.go`. Additional keys, `[dependencies]`, and a
manifest writer remain deferred and must grow additively.

### Project creation module

`purr/new.pcy` implements `new_project(name, library)` and the `NewResult`
union; `purr/new_probe.pcy` is the command-shaped test driver. The module
imports `manifest.pcy` for name validation and uses only `try_file_info`,
`try_create_directory`, `try_write_file`, and `try_remove` for filesystem work.
It tracks only paths it created, removes them in reverse order on failure, never
recurses, and reports cleanup notes. Modes honor the umask; no chmod exists.

The Go CLI does not dispatch `new` yet, so `purr/new_probe.pcy` built with the
Go pipeline remains the reference. `purr-pcy` dispatches `new` with the probe's
argument grammar, messages, and statuses. Any change to the templates or cleanup
behavior must update the both-pipeline tests in `cmd/purr/new_test.go` and the
purr-pcy parity tests in `cmd/purr/purr_pcy_test.go`.

### Percy `purr` sidecar

`purr.pcy` is the repository-root entry point; `purr/cli.pcy` parses argv and
imports `new.pcy`, `build.pcy`, and `project.pcy`. `purr/build.pcy` imports the
in-process loader, lexer, parser, checker, code generator, loader path helpers,
and `embedded.pcy`; it does not import the probe drivers that define their own
`main`. The binary name is always `purr-pcy`; the Go CLI remains the authoritative
`purr`. Help and usage name the program `purr` deterministically, never argv[0].

`purr-pcy` supports `help`, `new`, `build`, and `run`. `new -h`/`--help` prints
the dedicated project-scaffolding text from `purr/new.pcy`. The build path
mirrors the Go CLI's argument grammar, compiler/PATH/pkg-config discovery,
compiler argv, output validation, diagnostics, and statuses, with the two
documented deviations for combined stream order and signal status. `run`
shares that argument grammar, rejects `-o`, builds into a `purr-run.` temporary
directory, executes with `try_run`, propagates the child's status, and cleans up
non-recursively with notes. A signaled child therefore reports `128 + signal`
through `try_run`; the Go CLI's `ExitCode()` path reports `-1`, observed as 255
on POSIX. That difference is unavoidable while the documented `try_run` mapping
remains. Without a source argument, no output option, and a `purr.toml` in the
current directory, `build` and `run` enter binary project mode: they use the
manifest and `src/main.pcy`, write `target/<name>`, and run that executable.
Library projects report the approved deferred errors. The run feature and
project mode are implemented, the 9.8c parity suite is complete, and after the
9.9 freeze the binary built from `purr.pcy` takes over the `purr` name while
the Go CLI at `./cmd/purr` stays frozen as the bootstrap and testing
reference. Tests must keep the Go CLI behavior unchanged.

### Adding a built-in

A built-in is not registered in one place. Every item below must change together,
or the Go and Percy pipelines stop agreeing:

1. **SPEC.md** — Section 18.8 for the signature, argument rules, output, and errors;
   Section 16 when a deferred capability is removed; Section 19.3 for runtime ABI
   names; Section 28 for the Percy generator's expansion.
2. **Go checker** — `checker/symbols.go` (`BuiltinKind`), `checker/checker.go`
   (`builtinFunctions`, including signature/return type), and `checker/expr.go`
   (`checkCall`) for any argument rule that is not the generic exact-arity path.
3. **Percy checker** — `selfhost/checker/state.pcy` (`BuiltinKind`,
   `builtin_functions`, and any signature helper), plus
   `selfhost/checker/expr.pcy` (`check_call`).
4. **Semantic-model naming** — `selfhost/checker/semantics.pcy`
   (`builtin_kind_name`) and the Go mirror in
   `cmd/purr/semantics_differential_test.go` (`builtinKindName`).
5. **Runtime names** — `runtime/runtime.go` for the Go generator's constants and
   `selfhost/codegen/names.pcy` for the Percy generator's helpers.
6. **Runtime implementation** — `runtime/csrc/percy_runtime.h` and
   `runtime/csrc/percy_runtime.c`, kept under the MIT runtime notice. State
   flushing and failure behavior explicitly.
7. **Go generator** — `codegen/emit.go` (call dispatch and any expansion helper);
   add or use a runtime name constant rather than a literal C name.
8. **Percy generator** — `selfhost/codegen/expressions.pcy` (call dispatch and
   expansion) and `selfhost/codegen/names.pcy`.
9. **Harnesses and fixtures** — checker corpus valid and invalid uses, expression
   corpus, call/unit whole-unit cases, a runtime fixture when behavior is
   observable, and `selfhost/codegen/README.md` counts.
10. **Documentation** — `README.md`, `AGENTS.md`, and the relevant
    `selfhost/*/README.md`; update measured counts, never invent them.

A **prelude built-in** additionally changes `checker/prelude.go` and
`selfhost/checker/prelude.pcy` (source blocks, syntactic trigger, late
registration, and reserved names), both generators' helper emission, and the
prelude subset and whole-unit corpora.

A **runtime intrinsic** additionally changes the synthetic-module table and
`<runtime>` hook in both checkers, the runtime-only resolution gate, both
generators' lowering, the MIT `runtime/csrc` header helpers, and the runtime
module fixture. Intrinsic names are never reserved in user modules.

For a new **operator or primitive type**, this built-in checklist is not enough:
also update both lexers' token kinds and scans, both parsers' precedence or type
grammar, both checker type tables and operator rules, both code generators' C
type/operator lowering, the semantic-model type rendering, and the lexer,
parser, checker, semantic, expression, function, unit, and call corpora. A
prelude type follows the prelude rows above in addition to these.

None of the self-hosted components is part of the production pipeline: `purr
build` and `purr run` still use the Go lexer, parser, checker, generator, and
loader, which remain authoritative. The Percy-written driver and its POSIX
wrapper are test infrastructure for the fixpoint, not the production CLI. Their
deliberate differences (byte-oriented positions, an absolute-root Percy loader,
and no production driver integration) are
documented in SPEC.md Sections 25 to 28, `selfhost/lexer/README.md`,
`selfhost/parser/README.md`, `selfhost/checker/README.md`,
`selfhost/codegen/README.md`, `selfhost/loader/README.md`, and
`selfhost/driver/README.md`.

The C-level self-hosting fixpoint is reached, but full independence from Go is
not: Go still builds stage 1. The design of the zero-dependency native toolchain
(Goal 2) lives in `proposals/zero-dependency-toolchain.md`, and **Phase 10.0
approved its Section 14 answers, authorizing implementation**: the typed-IR,
stack-first, direct-ELF backend is approved; unsafe operations stay only in the
compiler-reserved `<runtime>` module; the first shippable GC is non-moving
conservative mark-sweep; the native runtime implements `%g` exactly; and after
native parity the Go source is removed entirely — an accepted loss of the
independent cross-check oracle, with the Go files frozen until that parity is
proven. The staged native-backend work is in that proposal's implementation
order; a complete standard library remains not designed. Do not claim
zero dependencies, a native fixpoint, or Go removal before the proposal's
staged evidence exists. Phase 10.1 then landed the backend's input
representation: the typed-IR scene under `selfhost/native` — shapes, binding
tables, labels, calls, blob data — with the whole-scene `check_scene` gate,
its deterministic validation messages, and a text renderer pinned through
`selfhost/native.pcy` and `cmd/purr/native_test.go` in both pipelines
(Go and Percy). Phase 10.2 then landed the skeleton slice: the x86-64
selector and the static ELF64 writer, with the proposal's internal stack
convention, two backend-internal raw instructions (a raw 64-bit load and a
raw syscall), in-place patching of every recorded fixup, and the program's
opening paragraph calling the entry and exiting through `exit_group`. The
skeleton's execution rung passed: `native elf <path>` writes a static ELF
with no C compiler, it runs with an empty `PATH`, `readelf -ld` shows two
program headers and no interpreter or dynamic section, the output and status
match the C pipeline's program, and both pipelines produce byte-identical
ELFs. The full instruction set — no lowering from language source yet — and
a standard library remain designed but unimplemented; relative work per the
proposal order continues. Phase 10.3 landed proposal step 2, scalar
functions and control flow: the selector now encodes every instruction form
— `add` through `whole_remainder`, `negate`, `equal`, `less_than`, `jump`,
`branch`, `label` — with no allocation. Division follows SPEC's defined
rules: a zero divisor jumps to a shared failure paragraph that writes the C
runtime's exact `purr: integer division by zero` text and exits with status
1, and a minus-one divisor takes the wrap path so `idiv` can never trap.
`push_instruction` now snapshots call argument arrays, so the scene stays an
immutable snapshot even when a builder reuses one array. The probe gained an
arithmetic-and-control-flow scene pinned byte for byte and `elf-arith` and
`elf-divzero` executables whose output, status, and standard error match the
C pipeline's builds of mirrored sources; both pipelines produce
byte-identical ELFs for all three scene kinds. Phase 10.4 then landed the
lowering: `selfhost/native/lower.pcy` reduces a checked single-module
program's semantic model into a `NativeScene` for the scalar,
allocation-free subset — int/bool/string values, arithmetic, comparisons
and boolean operators through branch patterns (with short-circuit `&&`/`||`),
the assignment family, if/while/for statements, calls, `print` with one
string argument, and `exit` — failing every unsupported construct with one
deterministic positioned message and never returning a partial scene. The
probe's `lower` mode runs real `.pcy` sources through the Percy-written
loader, checker, lowering, selector, and ELF writer; the corpus under
`tests/native` (15 programs plus one-pinned-message-per-failure sources in
`bad/`) matches the C pipeline's output, status, and stderr exactly, all
pipelines produce byte-identical ELFs and agreeing renders and messages, and
a pooled-literal render is pinned. Phase 10.5 then added the float slice:
`selfhost/native/floats.pcy` encodes decimal literal text into exact IEEE 754
binary64 bit patterns in pure integer Percy (16-bit limbs, one 53-bit
rounding against guard and sticky, deterministic failure past seventeen
significant digits, saturation and underflow exactly as the C toolchain's
text conversion), the scene grew nine SSE2 forms (constf, four float
arithmetic ops, three comparison ops, negatef) whose setCC/parity sequences
carry the C generators' exact NaN behavior, float results return in xmm0,
the probe gained a `float` mode differential-driven against Go's strconv
oracle, and the corpus covers the edges — tie rounding, NaN unorderedness,
infinity, the subnormal-to-zero edge, both increment positions — with
byte-identical C-pipeline parity and a pinned float render. Phase 10.6
landed the runtime foundations: every data blob is one object on the
8-byte-header ABI (the whole size in the header, then length, payload
pointer, payload — SPEC §19.10 and §30.7), with the string literals rebuilt
on it; `GetStatic`/`SetStatic` reach a blob's payload at a raw offset for
the heap cursor, which `runtime.pcy`'s `$heap`/`$map`/`$alloc` shells walk —
a lazy 256 MiB (later raised to 512 MiB in Phase 10.25) `mmap` bump allocator whose exhaustion, negative size, or
wrapped cursor halts with the C runtime's exact `purr: out of memory` at
status 1 — and `print(int)`/`print(bool)` lower into decimal-digit and
true/false helpers that byte-match the C pipeline, including INT64_MIN.
The print-kind prescan registers one print helper per argument kind, print
is supported with exactly one string, int, or bool argument, and the probe
gained `elf-oom` plus three pinned static-form validation messages.
Phase 10.7 then built the mark-sweep collector: every heap block is one
object behind a size-only header word, mark state lives in a
one-byte-per-word ledger beside the heap, and a conservative stack scan
(from the current stack pointer to the capture made at mapping time — a
new `StackPointer` form) marks live objects only where the ledger
certifies a header, so a stale value or an interior pointer can neither
corrupt nor miss memory. The sweep builds an address-ordered free list
whose adjacent holes coalesce, allocation goes first-fit through the list
(splitting tails) before trying the untouched cursor, and after one full
collection the exact `purr: out of memory` halt at status 1 is still the
end. The scene grew `RawStore64`, `RawLoad8`, `RawStore8`, and
`StackPointer`, and the string runtime — concat (`+`, `+=`), content
equality (`==`, `!=`), and the `length` built-in — is the first
user-reachable `$alloc` client. The probe gained `elf-gc`: six hundred
one-megabyte allocations through the collector, exit 0. Phase 10.8 then
landed exact `%g` float printing in the native backend: a FloatBits scene
form, a 16-bit-limb bigint substrate, sign/exponent/mantissa decomposition,
a six-digit significand with round-half-even, and fixed/scientific
formatting. A follow-up repair fixed the shared newline tail, the bigint
copy-back capacity when a remainder grows by a limb, and the roll's
six-digit clearing; `tests/native/printfloat.pcy` now pins zero and negative
zero, fixed/scientific thresholds, six-digit leading-zero values, rounding
edges, specials, and rational values against the C pipeline through both
pipelines' corpus runs. Phase 10.9 then raised the native selector's frame
capacity from fourteen parameters and fourteen locals to 4096 of each, later
raised to 8192 in Phase 10.33 when `try_run_capture` pushed the capture
builder past the old budget,
selecting disp32 ModRM forms and imm32 frame and argument adjustment only
when an offset needs them; small frames keep their previous bytes. The
corpus gained `bigframe.pcy` (sixty-four locals), `manyparams.pcy` (twenty
parameters), and a regenerated frame-limit probe (8194 locals after the Phase
10.33 budget raise). Phase 10.10 then
made the collector trace pointers stored inside heap objects:
`$mark_at` reports whether it newly marked a header, `$scan_heap` walks
marked payloads conservatively, and `$collect` repeats that pass to a
fixpoint before sweeping. The probe's `elf-gc-graph` scene is the white-box
proof — an outer payload's only edge to an inner block, with the stack scan
bypassed — and disabling the heap pass flips its `marked`/`changed` output
to `missed`/`nochange`. Phase 10.11 then added arrays of int, bool, float,
string, and reference elements: `new T[n]`, `a[i]`, element assignment and
compound assignment, `length(a)`, array parameters and returns, a separate
zeroed data buffer stored by block base, the exact C-runtime null/negative/
overflow/bounds failure texts on stderr, and the `BitsFloat` scene form for
float element loads. The corpus gained `arrays.pcy`, `arrayfns.pcy`, and
`arrayfail.pcy`, and the old "array values are not supported yet" fixture is
gone; string indexing, maps, structs, unions, field access, tag switches, and
arrays of composite values remain staged. Phase 10.12 then added byte values
as 0..255 int slots, byte arrays (element size one), unsigned comparisons,
`print(byte)`, the truncating `byte_from_int`, identity `int_from_byte`,
`string_to_bytes`, and the NUL-checking `bytes_to_string` with its exact
failure text. `bytes.pcy` and `bytesnul.pcy` joined the corpus. Phase 10.13
then added structs: the header-first aligned layout, struct literals,
member reads, `=` and compound member assignment, nested structs, struct
parameters/returns, and arrays of structs. Composite values now shape as
references (struct/union/map) or eight-byte tags (enum), so they can travel
through locals, parameters, returns, fields, and arrays while union/enum/map
operations stay unsupported. `structs.pcy` and `structmixed.pcy` joined the
corpus and the old "struct declarations are not supported yet" fixture is
gone. Phase 10.14 then made the lowering multi-module: every module's
functions register before bodies, imported names take an `m<id>_` scene
prefix, calls key through the resolved function's module, and each body
lowers against its own annotations and path. It also made `print` variadic
through raw helpers for each kind plus a space separator and one newline.
`tests/modules` runs through both probe pipelines, `printmany.pcy` covers
mixed multi-argument prints, and the old single-module and single-print
fixtures are gone. Union/enum construction and `match`, maps, string byte
indexing, stack alignment, register allocation, file/process built-ins, and
the native compiler stage also remain. Phases 10.15–10.19 then added enum
tags and enum `match`, flat union construction and union `match` with field
bindings, string indexing and `substring` with their exact failure texts,
descriptor-2 `eprint`, and `argv()` backed by an args blob saved from the
initial stack. The corpus gained `enums.pcy`, `unions.pcy`, `stringops.pcy`,
`stringindexfail.pcy`, `substringfail.pcy`, `eprint.pcy`, `argv.pcy`, and
`TestNativeProbeArgv`. The old enum/union/index/substring/eprint failure
fixtures are gone. Phase 10.20 then added maps: a header-plus-five-word
object, separate zeroed key/value buffers, a linear packed entry table with
capacity growth, string-content keys, inline value bytes, `$map_new`/
`$map_set`/`$map_at`/`$map_has`/`$map_length`, and the exact null and missing-
key failures. Lowering covers `new map[string]V`, indexing, insertion,
compound int/float updates, `has`, and `length`; `maps.pcy` and
`mapmissing.pcy` joined the corpus and the old map-creation fixture is gone.
The native lowering of `selfhost/driver.pcy` now reaches the file/process
built-in family. Phases 10.21–10.23 then added `try_current_directory`,
`try_read_file`, `try_write_file`, and `try_write_bytes` with their exact
recoverable failures, and Phases 10.24–10.25 added bitwise/shift operators,
bool equality, a stack-top capture for the conservative GC scan, and a
512 MiB heap for the native compiler's working set. Phase 10.26 reached the
native stage fixpoint: the C-pipeline probe compiles `selfhost/native.pcy`
into native stage 2, stage 2 compiles itself into byte-identical stage 3,
stage 3 reproduces stage 4, and stage 2 produces the same `arith` ELF as the
C pipeline and runs it with an empty `PATH`; `TestNativeProbeNativeFixpoint`
pins this. The native backend now compiles the C-emitting driver as well,
byte-identically to the Go reference for every fixture and `tests/modules`.
Phase 10.27 added the native `try_getenv`: the shell walks the `envp` vector
saved off the initial stack, stops on a null entry pointer, rejects empty and
`=`-containing names, and copies matching values into fresh GC-managed
strings. Phases 10.28–10.31 then added `try_create_directory`/`try_remove`,
`try_list_directory` through two `getdents64` passes and a bytewise insertion
sort, `try_file_info` with `lstat`/`stat`/`access`, and
`try_create_temp_directory` with prefix validation, `$TMPDIR` resolution,
`getrandom` suffix filling, and `mkdir(0700)` retries. Phase 10.32 added
`try_run`: a NUL-terminated argv, `fork`/`execve` with the inherited `envp`, a
close-on-exec pipe carrying the child's errno, and the exit/signal status
mapping. Phase 10.33 added `try_run_capture`: `/dev/null` stdin, separate
stdout/stderr pipes multiplexed with `poll`, 16 MiB per-stream buffers, NUL
and limit failures with exact text, and the `CapturedRunResult` status/stdout/
stderr shape. The native backend now also compiles `purr.pcy` into a working
native `purr` (`TestNativeProbeToolchain`: help, direct run, `new`, and project
run), and a natively built probe compiles the production toolchain. Phase 10.34
added the production native build path: the checker's reserved
`runtime/pcy/memory.pcy` and `runtime/pcy/io.pcy` helper paths, the
`write_executable`/`write_bytes` syscall writers, native lowering for every
runtime intrinsic, null-string normalization and the remaining string/map
compound assignments, and `purr build --native` / `purr run --native`, which
write a static ELF through the native backend with no C compiler
(`TestPurrPcyNativeBuild` and `TestRuntimeIoWriterBothPipelines`). The Percy
`purr` now uses the native path by default: `purr/build.pcy` no longer imports
the C runtime or C code generator, the CLI help states the native backend, and
the C-only fake-compiler, CC/GC discovery, pkg-config, and compiler-diagnostic
parity cases are retired with that deviation. The Go C pipeline remains the
frozen reference and the stage-1 bootstrap. Phase 10.35 added
`TestPercyPurrZeroDependency`, the Section 8 rung-6 check: the shipped `purr`
runs with a PATH holding only itself and `CC=/nonexistent`, builds and runs all
fixtures and `tests/modules/`, produces static ELFs (`readelf -d`), and reaches
a byte-identical native self-build fixpoint. Stack alignment, register
allocation, the Go-free bootstrap artifact, and the Goal 3 ecosystem remain.
Phase 10.36 aligned the native stack to the System V contract: frames round
their local bytes up to sixteen and odd-argument calls carry a below-argument
pad, with `TestNativeProbeAlign`'s white-box `elf-align` scene and its
disable-either-fix sensitivity check. Phase 10.37 added the native fatal
`read_file`: the `$read_file` shell's open/fstat/read path, the 256 MiB limit,
the directory/non-regular fseek-style failures, the libc-cased fatal reasons,
and the `TestNativeProbeReadFileFatal` both-pipeline comparison over regular,
empty, directory, missing, and oversized paths. Phase 10.38 added the first
register-allocation slice: a straight-line `rax` cache in the selector, with
invalidation at joins, calls, and memory writes, which shrank the native
compiler's own ELF by about 10.4%. Phase 10.39 added loop control: `break;` and `continue;` across the lexers,
parsers, checkers, both generators, and the native lowering, with
`InvalidLoopControl` outside a loop. It also changed match codegen from a C
`switch` to an `if`/`else if` chain so a `break` inside an arm targets the
enclosing loop; `tests/break_continue.pcy` and `tests/native/loopcontrol.pcy`
cover it. Phase 10.40 added `push(array, value)`, the first growable-array operation: the
array object gained a capacity word after its data pointer (the earlier fields
keep their offsets), growth doubles the buffer and copies the live payload, Go
and Percy generators emit `pcy_rt_array_push` with the element as a compound
literal, and the native selector's `$array_push` shell stores one or eight bytes
through the same capacity word. `tests/append.pcy`, `tests/native/push.pcy`, and
the checker, unit, and native corpora cover it. Phase 10.41 then renamed the
language, extension, runtime prefixes, Go module, documentation, website, and
profile from Kitten to Percy: `.pcy` files, `pcy_*` and `pcy_rt_*` identifiers,
`percy_runtime.{h,c}`, the `percy` module, and `runtime/pcy`; the tool remains
`purr`. Full register allocation (callee-saved registers and liveness), the
Go-free bootstrap artifact, and the Goal 3 ecosystem remain. The release-flow
test skips on a dirty checkout because it clones committed HEAD.

## Roadmap

1. Go-based compiler frontend and C backend. **Done** (Phases 1–2C).
2. A working, tested native compiler for the current language, including the
   language foundations self-hosting needs. **In progress** (Phase 3A added
   dynamic strings; Phase 3B added dynamic arrays; Phase 3C added file I/O and
   modules; Phase 3D added enums and tagged unions; Phase 3E added typed maps;
   Phase 3F added string byte access; Phase 3G added a Percy-written lexer
   checked against the Go lexer, without replacing it; Phase 3H made strings
   length-carrying so that scanning is linear; Phase 4A added a Percy-written
   parser checked against the Go parser, without replacing it; Phase 4B added a
   Percy-written semantic checker checked against the Go checker, without
   replacing it; Phase 4C exposed the checker's resolved semantic model for a
   future code generator; Phase 4D.1 began the Percy-written C generator with
   its naming, C type mapping, string escaping, and bounded output accumulation;
   Phase 4D.2 added its declaration emitter, checked against the reference
   generator byte for byte; Phase 4D.3 added its core expression emitter, checked
   node by node against the reference; Phase 4D.4 added string literal pooling,
   checked against the reference's pooled declarations; Phase 4D.5 added the
   statement and function-body emitter, checked against the reference's own
   definitions; Phase 4D.6 added the entry point rule and the complete translation
   unit assembly, checked against the reference's whole output; Phase 4D.7 added
   function calls and built-ins, with generated programs compiled, run, and compared
   against the reference's; Phase 4D.8 added member access, qualified variant
   references, struct and variant literals, array and map creation, and indexing;
   Phase 4D.9 added assignment, map-entry insertion, compound assignment,
   increment/decrement, and string append, so every expression kind is emitted and
   all nine top-level fixtures now compile and run through the Percy backend;
   Phase 4E.1 added the Percy-written module loader for absolute roots, with the
   same order, ids, paths, and diagnostics as Go over the loader corpus, and a
   fully Percy-written path through `tests/modules/`. Relative roots were
   deferred then because there was no working-directory primitive, and were
   accepted in Phase 9.10; Phase 5.1
   added `argv()` in both pipelines and the `argc`/`argv`-capturing C entry point;
   Phase 5.2 added `eprint(...)`, the flushing stderr helpers, and separate-stream
   runtime coverage; Phase 5.3 added `exit(status)`, a shared flush-and-exit path,
   and exact status/stream runtime coverage; Phase 5.6 added the Percy-written
   driver and temporary POSIX-shell wrapper, with byte-identical C and native
   fixture execution under GCC and Clang; Phase 5.7 reached the C-level fixpoint;
   Phase 5.4c brought `try_read_file` and `ReadFileResult` to the Percy-written
   checker and generator; Phase 5.5 closed loader read-error parity; Phase 8.1
   made integer arithmetic and decimal literal ranges defined in both pipelines
   and the C runtime; Phase 8.2 added bitwise/shift operators and hex/binary
   literals; Phase 8.3a freed the `byte` identifier by renaming it to `octet`
   in source and fixtures; Phase 8.3b added the `byte` type, `byte[]`, and
   explicit byte/string conversions; Phase 8.4 added recoverable file and
   directory operations and their on-use result unions; Phase 8.5 added
   recoverable process launch and `RunResult`; Phase 8.6 added the current
   directory query and `CurrentDirectoryResult`; Phase 8.7 added the
   compiler-reserved `<runtime>` module and its intrinsic mechanism).
3. Rewrite the compiler in Percy and verify it through an actual bootstrap. This
   is independence from Go. **C-level fixpoint reached** (5.7): Go builds stage 1,
   stage 1 builds stage 2, stage 2 builds stage 3, and the C for `driver.pcy` is
   byte-identical across Go and all stages. Full independence remains open because
   Go still builds stage 1; a Go-free bootstrap artifact remains open work.
4. Explore an LLVM IR or native assembly/object-code backend, and eventually move
   the runtime off C as well. This is independence from C. **Authorized** (Phase
   10.0): the typed-IR/stack-first/direct-ELF design is approved and
   implementation proceeds in the proposal's staged order. Phase 10.1 landed
   the typed-IR scene; Phase 10.2 the static-ELF skeleton; Phase 10.3 scalar
   arithmetic and control flow in the selector; Phase 10.4 the lowering of
   checked programs into scenes and corpus executable parity with the C
   pipeline; Phase 10.5 floats end to end (encoder to SSE2); the GC, the
   native runtime, and the native build path are implemented. Full register
   allocation, broader platforms, and the standard library remain.
5. Grow a broader standard library and an ecosystem for command-line tools first,
   then GUI, servers, and games. This is independence from other languages'
   ecosystems, and it includes the whole `purr` toolchain: one-command install of
   the toolchain, the language server and editor and shell completion, `purr new`
   scaffolding for binaries and libraries, dependency management, the registry,
   publishing, and the standard library that makes them useful.
6. One-line installer (rustup-style) that installs `purr` and the whole
   toolchain: components, update model, completion, and language server. **Planned,
   not designed**; design it after the zero-dependency path is designed so it does
   not advertise a toolchain that still needs Go, C, or libgc.

The initial compiler is written in Go. The long-term goal is for Percy to
compile its own compiler, to stop depending on C, and to stand on its own
tooling. Those are three separate goals that do not depend on each other;
`VISION.md` states them in full. This is a direction for future design, not
permission to add language features or phases early.
