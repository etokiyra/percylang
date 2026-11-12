package main

// Pinned tests for the native backend's typed-IR scene, driven by
// selfhost/native.pcy. The probe builds the Phase 10.1 scene corpus (the
// print-hello skeleton target and a growth scene) plus the Phase 10.3
// arithmetic-and-control-flow scene and its divide-failure sibling, checks
// them with check_scene, and renders them with the text renderer; their
// output and every validation message are pinned literally here. Since Phase
// 10.4 the probe also lowers real sources: the `lower` mode loads and checks
// a program with the Percy-written loader and checker and reduces its
// scalar subset into a scene, whose rendering and executable the tests pin
// and compare with the C pipeline's behavior for the same source. The ELF
// tests execute the produced executables and compare them with the C
// pipeline's behavior for mirrored sources. See selfhost/native/README.md
// for the scene's contract and the spelling rules.

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func buildNativeProbe(t *testing.T) string {
	t.Helper()
	requireToolchain(t)
	return cachedBuild(t, "native-probe", cachedBuildHash(t, "native-probe", []string{
		repoPath(t, "..", "..", "selfhost"),
		repoPath(t, "..", "..", "runtime", "csrc"),
	}), func(executable string) error {
		var diagnostics bytes.Buffer
		if err := buildExecutable(repoPath(t, "..", "..", "selfhost", "native.pcy"), executable, &diagnostics); err != nil {
			return err
		}
		if warnings := strings.TrimSpace(diagnostics.String()); warnings != "" {
			return fmt.Errorf("C diagnostics while building the native probe:\n%s", warnings)
		}
		return nil
	})
}

// The exact rendering of the skeleton target: two data blobs, a runtime
// print routine that loads the string object's length and pointer and makes
// two write syscalls, and an entry that binds the literal and calls it.
const wantHelloScene = `scene
data $blob0: 5 bytes 104 101 108 108 111
data $blob1: 1 bytes 10
entry 1 @main
fn @$print (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b4: int binding
  b5: ref binding
  b6: ref binding
  b7: int binding
  b8: int binding
  b1: int = raw_load64 b0 8
  b2: ref = raw_load64 b0 16
  b3: int = const 1
  b4: int = raw_syscall syscall b3 b3 b2 b1
  b5: ref = data $blob1
  b6: ref = raw_load64 b5 16
  b7: int = const 1
  b8: int = raw_syscall syscall b7 b7 b6 b7
  return
fn @main () -> void
  b0: ref binding
  b0: ref = data $blob0
  call @0 b0
  return

`

// The exact rendering of the Phase 10.3 arithmetic scene: the shared print
// routine, a step helper whose body multiplies and subtracts, and a main
// that loops a call through defined divide, remainder, and add, then
// compares twice and prints one of three literals.
const wantArithScene = `scene
data $blob0: 1 bytes 10
data $blob1: 5 bytes 108 97 114 103 101
data $blob2: 5 bytes 115 109 97 108 108
data $blob3: 6 bytes 110 101 103 103 101 100
entry 2 @main
fn @$print (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b4: int binding
  b5: ref binding
  b6: ref binding
  b7: int binding
  b8: int binding
  b1: int = raw_load64 b0 8
  b2: ref = raw_load64 b0 16
  b3: int = const 1
  b4: int = raw_syscall syscall b3 b3 b2 b1
  b5: ref = data $blob0
  b6: ref = raw_load64 b5 16
  b7: int = const 1
  b8: int = raw_syscall syscall b7 b7 b6 b7
  return
fn @step (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b1: int = const 3
  b2: int = multiply b0 b1
  b3: int = const 1
  b4: int = subtract b2 b3
  return b4
fn @main () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: bool binding
  b10: ref binding
  b11: ref binding
  b0: int = const 47
  b1: int = const 0
  b2: int = const 5
  b3: int = const 1
  label %0
  b4: bool = less_than b1 b2
  branch b4 %1 %2
  label %1
  b5: int = call @1 b0
  b6: int = whole_divide b5 b2
  b7: int = whole_remainder b0 b2
  b0: int = add b6 b7
  b1: int = add b1 b3
  jump %0
  label %2
  b8: int = const 19
  b9: bool = less_than b8 b0
  branch b9 %3 %4
  label %3
  b10: ref = data $blob1
  call @0 b10
  jump %5
  label %4
  b11: ref = data $blob2
  call @0 b11
  label %5
  b8: int = negate b3
  b5: int = whole_divide b0 b8
  b6: int = whole_remainder b0 b8
  b9: bool = less_than b5 b6
  branch b9 %6 %7
  label %6
  b10: ref = data $blob3
  call @0 b10
  label %7
  return

`

func TestNativeProbeHelloScene(t *testing.T) {
	executable := buildNativeProbe(t)
	result := runExternal(t, executable, nil, t.TempDir(), nil)
	if result.status != 0 || result.stderr != "" {
		t.Fatalf("hello scene: status %d, stderr %q", result.status, result.stderr)
	}
	if result.stdout != wantHelloScene {
		t.Fatalf("hello scene rendering differs:\n--- got ---\n%s--- want ---\n%s", result.stdout, wantHelloScene)
	}
}

// The growth scene must exercise every table-doubling path: twelve functions
// (more than the first function allocation), nine blobs (more than the first
// blob allocation), and seventeen bindings per function (more than the first
// binding allocation). The structural counts here are pinned next to spot
// lines, because the whole 444-line rendering is mechanical.
func TestNativeProbeBigScene(t *testing.T) {
	executable := buildNativeProbe(t)
	result := runExternal(t, executable, []string{"big"}, t.TempDir(), nil)
	if result.status != 0 || result.stderr != "" {
		t.Fatalf("big scene: status %d, stderr %q", result.status, result.stderr)
	}
	lines := strings.Split(strings.TrimRight(result.stdout, "\n"), "\n")

	// One scene header, nine data blob lines, one entry line, and twelve
	// functions of a signature line, seventeen binding lines, and eighteen
	// instruction lines each: 1 + 9 + 1 + 12*36.
	if len(lines) != 1+9+1+12*(1+17+18) {
		t.Fatalf("big scene has %d lines", len(lines))
	}
	if lines[0] != "scene" {
		t.Fatalf("big scene does not open with scene: %q", lines[0])
	}
	for _, want := range []string{
		"data $blob0: 1 bytes 97",
		"data $blob8: 1 bytes 97",
		"entry 0 @f0",
		"fn @f0 () -> int",
		"  b16: int = const 16",
		"  return b16",
		"fn @f11 () -> int",
		"  b16: int = const 203",
		"  return b16",
	} {
		if !strings.Contains(result.stdout, want+"\n") {
			t.Fatalf("big scene misses %q", want)
		}
	}
	for _, unexpected := range []string{
		"data $blob9",
		"fn @f12",
	} {
		if strings.Contains(result.stdout, unexpected+"\n") {
			t.Fatalf("big scene unexpectedly contains %q", unexpected)
		}
	}
}

func TestNativeProbeFailures(t *testing.T) {
	executable := buildNativeProbe(t)
	cases := []struct {
		mode string
		want string
	}{
		{"no-label", "error function @main: instruction 0: label 0 is out of range (the function defines 0 labels)\n"},
		{"bad-binding", "error function @main: instruction 0: target binding 0 is out of range (the function has 0 bindings)\n"},
		{"bad-call-count", "error function @main: instruction 0: the call passes 0 arguments, but @print takes 1\n"},
		{"bad-call-shape", "error function @main: instruction 0: argument 0 binding 0 has bool, expected ref\n"},
		{"bad-entry", "error entry 5 is out of range (the scene has 1 functions)\n"},
		{"undefined-label", "error function @main: instruction 1: true label 1 is out of range (the function defines 1 labels)\n"},
		{"bad-static-blob", "error function @main: instruction 0: blob 9 is out of range (the scene has 1 blobs)\n"},
		{"bad-static-offset", "error function @main: instruction 0: offset 60 does not fit blob $blob0's 32 bytes\n"},
		{"bad-static-value", "error function @main: instruction 0: value binding 0 has bool, expected int\n"},
		// An unknown mode falls through to the empty scene, whose entry
		// reference is the first failure the checker reports.
		{"wat", "error entry 0 is out of range (the scene has 0 functions)\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.mode, func(t *testing.T) {
			result := runExternal(t, executable, []string{testCase.mode}, t.TempDir(), nil)
			if result.status != 1 {
				t.Fatalf("status %d, want 1", result.status)
			}
			if result.stdout != testCase.want {
				t.Fatalf("stdout differs:\n--- got ---\n%s--- want ---\n%s", result.stdout, testCase.want)
			}
			if result.stderr != "" {
				t.Fatalf("stderr %q, want empty", result.stderr)
			}
		})
	}
}

func TestNativeProbeTooManyArguments(t *testing.T) {
	executable := buildNativeProbe(t)
	result := runExternal(t, executable, []string{"one", "two", "three", "four"}, t.TempDir(), nil)
	if result.status != 2 {
		t.Fatalf("status %d, want 2", result.status)
	}
	if result.stderr != "native: expected at most three arguments\n" {
		t.Fatalf("stderr %q", result.stderr)
	}
	if result.stdout != "" {
		t.Fatalf("stdout %q, want empty", result.stdout)
	}

	unknown := runExternal(t, executable, []string{"wat", "out"}, t.TempDir(), nil)
	if unknown.status != 2 {
		t.Fatalf("unknown status %d, want 2", unknown.status)
	}
	if unknown.stderr != "native: unknown mode wat\n" {
		t.Fatalf("unknown stderr %q", unknown.stderr)
	}
}

// TestNativeProbeSkeleton writes the skeleton target's native ELF with no C
// compiler, then executes it: the output and status must match the C
// pipeline's program for the same source, the ELF must be static (no
// interpreter, no dynamic section), and repeated productions must be
// byte-identical.
func TestNativeProbeSkeleton(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()

	native := filepath.Join(directory, "hello-native")
	written := runExternal(t, executable, []string{"elf", native}, directory, nil)
	if written.status != 0 || written.stderr != "" || written.stdout != "built "+native+"\n" {
		t.Fatalf("skeleton write: status %d, stdout %q, stderr %q", written.status, written.stdout, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	run := runExternal(t, native, nil, directory, []string{"PATH=/nonexistent"})
	if run.status != 0 || run.stderr != "" || run.stdout != "hello\n" {
		t.Fatalf("native run: status %d, stdout %q, stderr %q", run.status, run.stdout, run.stderr)
	}

	// A second production is byte-identical: the selector, the layout, and
	// the ELF writer are deterministic.
	again := filepath.Join(directory, "hello-native-again")
	if written := runExternal(t, executable, []string{"elf", again}, directory, nil); written.status != 0 {
		t.Fatalf("second write status %d", written.status)
	}
	first, err := os.ReadFile(native)
	if err != nil {
		t.Fatalf("reading the first ELF: %v", err)
	}
	secondFile, err := os.ReadFile(again)
	if err != nil {
		t.Fatalf("reading the second ELF: %v", err)
	}
	if string(first) != string(secondFile) {
		t.Fatalf("two native productions differ in %d bytes", len(first))
	}

	// The ELF is static: no interpreter, no dynamic section, no needed
	// library, and two PT_LOAD program headers.
	if readelf, err := exec.LookPath("readelf"); err == nil {
		listing := exec.Command(readelf, "-ld", native)
		var out bytes.Buffer
		listing.Stdout = &out
		listing.Stderr = &out
		_ = listing.Run()
		text := out.String()
		if strings.Contains(text, "INTERP") || strings.Contains(text, "NEEDED") || strings.Contains(text, "Dynamic section") {
			t.Fatalf("native ELF is not static:\n%s", text)
		}
		if !strings.Contains(text, "There are 2 program headers") {
			t.Fatalf("native ELF's program headers got not read back:\n%s", text)
		}
	}

	// The C pipeline's program for the same source behaves identically.
	cSource := filepath.Join(directory, "hello-c.pcy")
	if err := os.WriteFile(cSource, []byte("fn main() {\n    print(\"hello\");\n}\n"), 0o644); err != nil {
		t.Fatalf("writing the C-parity source: %v", err)
	}
	cRun := runExternal(t, goPurrReference(t), []string{"run", cSource}, directory, nil)
	if cRun.status != 0 || cRun.stdout != "hello\n" || cRun.stderr != "" {
		t.Fatalf("C reference run: status %d, stdout %q, stderr %q", cRun.status, cRun.stdout, cRun.stderr)
	}
}

// TestNativeProbeArithScene pins the arithmetic scene's rendering.
func TestNativeProbeArithScene(t *testing.T) {
	executable := buildNativeProbe(t)
	result := runExternal(t, executable, []string{"arith"}, t.TempDir(), nil)
	if result.status != 0 || result.stderr != "" {
		t.Fatalf("arith scene: status %d, stderr %q", result.status, result.stderr)
	}
	if result.stdout != wantArithScene {
		t.Fatalf("arith scene rendering differs:\n--- got ---\n%s--- want ---\n%s", result.stdout, wantArithScene)
	}
}

// TestNativeProbeArith writes the arithmetic-and-control-flow executable
// with no C compiler, runs it with an empty PATH, and checks its output
// against the C pipeline's build of the mirrored source; it also confirms a
// repeated production is byte-identical.
func TestNativeProbeArith(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()

	native := filepath.Join(directory, "arith-native")
	written := runExternal(t, executable, []string{"elf-arith", native}, directory, nil)
	if written.status != 0 || written.stderr != "" || written.stdout != "built "+native+"\n" {
		t.Fatalf("arith write: status %d, stdout %q, stderr %q", written.status, written.stdout, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	run := runExternal(t, native, nil, directory, []string{"PATH=/nonexistent"})
	if run.status != 0 || run.stderr != "" || run.stdout != "small\nnegged\n" {
		t.Fatalf("native arith run: status %d, stdout %q, stderr %q", run.status, run.stdout, run.stderr)
	}

	again := filepath.Join(directory, "arith-native-again")
	if written := runExternal(t, executable, []string{"elf-arith", again}, directory, nil); written.status != 0 {
		t.Fatalf("second write status %d", written.status)
	}
	first, err := os.ReadFile(native)
	if err != nil {
		t.Fatalf("reading the first ELF: %v", err)
	}
	secondFile, err := os.ReadFile(again)
	if err != nil {
		t.Fatalf("reading the second ELF: %v", err)
	}
	if string(first) != string(secondFile) {
		t.Fatalf("two arith productions differ in %d bytes", len(first))
	}

	cSource := filepath.Join(directory, "arith-c.pcy")
	source := `fn step(int value) -> int {
    return value * 3 - 1;
}
fn main() {
    int total = 47;
    int round = 0;
    while (round < 5) {
        total = step(total) / 5 + total % 5;
        round = round + 1;
    }
    if (total > 19) {
        print("large");
    } else {
        print("small");
    }
    int under = -1;
    int flipped = total / under;
    int remains = total % under;
    if (flipped < remains) {
        print("negged");
    }
}
`
	if err := os.WriteFile(cSource, []byte(source), 0o644); err != nil {
		t.Fatalf("writing the C-parity source: %v", err)
	}
	cRun := runExternal(t, goPurrReference(t), []string{"run", cSource}, directory, nil)
	if cRun.status != 0 || cRun.stdout != "small\nnegged\n" || cRun.stderr != "" {
		t.Fatalf("C reference arith run: status %d, stdout %q, stderr %q", cRun.status, cRun.stdout, cRun.stderr)
	}
}

// TestNativeProbeDivzero writes the divide-failure executable with no C
// compiler and runs it: zero divisors must halt with the C runtime's exact
// text, standard error only, and status 1, matching the C pipeline's build
// of the mirrored source.
func TestNativeProbeDivzero(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()

	native := filepath.Join(directory, "divzero-native")
	written := runExternal(t, executable, []string{"elf-divzero", native}, directory, nil)
	if written.status != 0 || written.stderr != "" || written.stdout != "built "+native+"\n" {
		t.Fatalf("divzero write: status %d, stdout %q, stderr %q", written.status, written.stdout, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	run := runExternal(t, native, nil, directory, []string{"PATH=/nonexistent"})
	if run.status != 1 || run.stderr != "purr: integer division by zero\n" || run.stdout != "" {
		t.Fatalf("native divzero run: status %d, stdout %q, stderr %q", run.status, run.stdout, run.stderr)
	}

	cSource := filepath.Join(directory, "divzero-c.pcy")
	source := "fn main() {\n    int zero = 0;\n    print(5 / zero);\n}\n"
	if err := os.WriteFile(cSource, []byte(source), 0o644); err != nil {
		t.Fatalf("writing the C-parity source: %v", err)
	}
	cRun := runExternal(t, goPurrReference(t), []string{"run", cSource}, directory, nil)
	if cRun.status != 1 || cRun.stdout != "" || cRun.stderr != "purr: integer division by zero\n" {
		t.Fatalf("C reference divzero run: status %d, stdout %q, stderr %q", cRun.status, cRun.stdout, cRun.stderr)
	}
}

// goPurrReferenceExecutive building the test reference each time is wasteful;
// this helper builds the reference once per test.
func goPurrReference(t *testing.T) string {
	t.Helper()
	return buildGoPurr(t)
}

// The Phase 10.4 pool render, pinned: equal literals share one blob, the
// helper's newline blob comes first, and the helper is named "$print" so no
// user name can collide with it.
const wantPoolScene = `scene
data $blob0: 8 bytes 32 32 32 32 32 32 32 32
data $blob1: 48 bytes 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255
data $blob2: 16 bytes 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255
data $blob3: 6 bytes 112 117 114 114 58 32
data $blob4: 1 bytes 10
data $blob5: 18 bytes 112 117 114 114 58 32 97 114 114 97 121 32 105 110 100 101 120 32
data $blob6: 28 bytes 32 105 115 32 111 117 116 32 111 102 32 114 97 110 103 101 32 102 111 114 32 108 101 110 103 116 104 32
data $blob7: 31 bytes 97 114 114 97 121 32 108 101 110 103 116 104 32 99 97 110 110 111 116 32 98 101 32 110 101 103 97 116 105 118 101
data $blob8: 35 bytes 97 114 114 97 121 32 101 108 101 109 101 110 116 32 115 105 122 101 32 109 117 115 116 32 98 101 32 112 111 115 105 116 105 118 101
data $blob9: 29 bytes 97 114 114 97 121 32 97 108 108 111 99 97 116 105 111 110 32 105 115 32 116 111 111 32 108 97 114 103 101
data $blob10: 38 bytes 97 114 114 97 121 32 97 99 99 101 115 115 32 111 110 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob11: 32 bytes 108 101 110 103 116 104 32 111 102 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob12: 32 bytes 97 112 112 101 110 100 32 116 111 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob13: 46 bytes 112 117 114 114 58 32 98 121 116 101 32 97 114 114 97 121 32 99 111 110 116 97 105 110 115 32 97 32 78 85 76 32 98 121 116 101 32 97 116 32 105 110 100 101 120 32
data $blob14: 18 bytes 98 121 116 101 32 97 114 114 97 121 32 105 115 32 110 117 108 108
data $blob15: 32 bytes 98 121 116 101 32 97 114 114 97 121 32 101 108 101 109 101 110 116 32 115 105 122 101 32 105 115 32 110 111 116 32 49
data $blob16: 19 bytes 112 117 114 114 58 32 115 116 114 105 110 103 32 105 110 100 101 120 32
data $blob17: 22 bytes 112 117 114 114 58 32 115 117 98 115 116 114 105 110 103 32 114 97 110 103 101 32
data $blob18: 18 bytes 112 117 114 114 58 32 115 104 105 102 116 32 99 111 117 110 116 32
data $blob19: 17 bytes 32 105 115 32 111 117 116 115 105 100 101 32 48 46 46 54 51
data $blob20: 2 bytes 46 46
data $blob21: 1 bytes 32
data $blob22: 4 bytes 116 114 117 101
data $blob23: 5 bytes 102 97 108 115 101
data $blob24: 1 bytes 48
data $blob25: 1 bytes 10
data $blob26: 1 bytes 32
data $blob27: 4 bytes 115 97 109 101
data $blob28: 5 bytes 111 116 104 101 114
entry 0 @main
fn @main () -> void
  b0: ref binding
  b1: ref binding
  b2: ref binding
  b3: ref binding
  b4: ref binding
  b5: ref binding
  b1: ref = data $blob27
  b0: ref = copy b1
  b3: ref = data $blob27
  b2: ref = copy b3
  b5: ref = data $blob28
  b4: ref = copy b5
  call @41 b0
  call @41 b2
  call @41 b4
fn @$mmap (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b1: int = const 0
  b2: int = const 3
  b3: int = const 34
  b4: int = const 9
  b5: int = raw_syscall syscall b4 b1 b0 b2 b3
  b6: bool = less_than b5 b1
  branch b6 %0 %1
  label %0
  fail_oom
  label %1
  return b5
fn @$map () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b0: int = const 536870912
  b1: int = const 67108872
  b2: int = call @1 b0
  b3: int = call @1 b1
  b4: int = get_static $blob0 24
  b5: int = const 
  b6: int = add b2 b0
  set_static $blob1 24 b2
  set_static $blob1 32 b2
  set_static $blob1 40 b6
  set_static $blob1 48 b5
  set_static $blob1 56 b5
  set_static $blob1 64 b5
  set_static $blob2 24 b3
  set_static $blob2 32 b4
  return
fn @$pad8 (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: bool binding
  b1: int = const 15
  b2: int = const 8
  b3: int = const 16
  b4: int = add b0 b1
  b5: int = whole_divide b4 b2
  b6: int = multiply b5 b2
  b7: bool = less_than b6 b3
  branch b7 %0 %1
  label %0
  b6: int = copy b3
  label %1
  return b6
fn @$byte_set1 (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = const 1
  raw_store8 b6 0 b7
  return
fn @$byte_set0 (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = const 0
  raw_store8 b6 0 b7
  return
fn @$search (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: bool binding
  b11: int binding
  b12: bool binding
  b13: int binding
  b14: int binding
  b1: int = const 0
  b2: int = get_static $blob1 48
  b3: int = copy b1
  b4: int = copy b2
  b5: int = const 16
  label %0
  b6: bool = less_than b4 b1
  branch b6 %9 %1
  label %1
  b7: int = raw_load64 b4 0
  b8: bool = less_than b7 b0
  branch b8 %2 %3
  label %2
  b3: int = copy b4
  b4: int = raw_load64 b4 8
  jump %0
  label %3
  b9: int = raw_load64 b4 8
  b10: bool = equal b3 b1
  branch b10 %4 %5
  label %4
  set_static $blob1 48 b9
  jump %6
  label %5
  raw_store64 b3 8 b9
  label %6
  b11: int = subtract b7 b0
  b12: bool = less_than b11 b5
  branch b12 %8 %7
  label %7
  b13: int = add b4 b0
  raw_store64 b13 0 b11
  b14: int = get_static $blob1 48
  raw_store64 b13 8 b14
  set_static $blob1 48 b13
  jump %8
  label %8
  raw_store64 b4 0 b0
  call @4 b4
  return b4
  label %9
  return b1
fn @$bump (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b1: int = const 0
  b2: int = get_static $blob1 32
  b3: int = get_static $blob1 40
  b4: int = add b2 b0
  b5: bool = less_than b3 b4
  branch b5 %0 %1
  label %1
  set_static $blob1 32 b4
  raw_store64 b2 0 b0
  call @4 b2
  return b2
  label %0
  return b1
fn @$mark_at (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: bool binding
  b14: int binding
  b1: int = const 0
  b2: int = get_static $blob1 24
  b3: bool = less_than b0 b2
  branch b3 %0 %1
  label %1
  b4: int = get_static $blob1 40
  b5: bool = less_than b4 b0
  branch b5 %0 %2
  label %2
  b6: int = const 8
  b7: int = subtract b0 b2
  b8: int = whole_divide b7 b6
  b9: int = get_static $blob2 24
  b10: int = add b9 b8
  b11: int = raw_load8 b10 0
  b12: int = const 1
  b13: bool = equal b11 b12
  branch b13 %3 %0
  label %3
  b14: int = const 2
  raw_store8 b10 0 b14
  b1: int = const 1
  label %0
  return b1
fn @$ledger_at (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b1: int = const 0
  b2: int = get_static $blob1 24
  b3: bool = less_than b0 b2
  branch b3 %0 %1
  label %1
  b4: int = get_static $blob1 40
  b5: bool = less_than b4 b0
  branch b5 %0 %2
  label %2
  b6: int = const 8
  b7: int = subtract b0 b2
  b8: int = whole_divide b7 b6
  b9: int = get_static $blob2 24
  b10: int = add b9 b8
  b1: int = raw_load8 b10 0
  label %0
  return b1
fn @$scan_roots () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b0: int = get_static $blob2 32
  b1: int = stack_pointer
  b2: int = const 7
  b3: int = const 8
  b4: int = subtract b1 b2
  b4: int = whole_divide b4 b3
  b4: int = multiply b4 b3
  label %0
  b5: bool = less_than b4 b0
  branch b5 %1 %2
  label %1
  b6: int = raw_load64 b4 0
  b7: int = call @8 b6
  b4: int = add b4 b3
  jump %0
  label %2
  return
fn @$scan_heap () -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: bool binding
  b14: int binding
  b15: int binding
  b16: bool binding
  b17: int binding
  b18: int binding
  b19: bool binding
  b0: int = const 0
  b1: int = const 2
  b2: int = const 8
  b3: int = get_static $blob1 24
  b4: int = get_static $blob1 32
  b5: int = copy b3
  b6: int = copy b0
  label %0
  b19: bool = less_than b5 b4
  branch b19 %1 %2
  label %1
  b7: int = raw_load64 b5 0
  b8: int = subtract b5 b3
  b9: int = whole_divide b8 b2
  b10: int = get_static $blob2 24
  b11: int = add b10 b9
  b12: int = raw_load8 b11 0
  b13: bool = equal b12 b1
  branch b13 %3 %4
  label %3
  b14: int = add b5 b2
  b15: int = add b5 b7
  label %5
  b16: bool = less_than b14 b15
  branch b16 %6 %4
  label %6
  b17: int = raw_load64 b14 0
  b18: int = call @8 b17
  b6: int = add b6 b18
  b14: int = add b14 b2
  jump %5
  label %4
  b5: int = add b5 b7
  jump %0
  label %2
  return b6
fn @$free_block (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b2: int = const 0
  b3: int = get_static $blob1 48
  b4: int = get_static $blob1 56
  b5: int = get_static $blob1 64
  b6: bool = equal b5 b0
  branch b6 %0 %1
  label %0
  b7: int = raw_load64 b4 0
  b8: int = add b7 b1
  raw_store64 b4 0 b8
  b9: int = add b0 b1
  set_static $blob1 64 b9
  call @5 b0
  return
  label %1
  b10: int = add b0 b1
  raw_store64 b0 8 b3
  set_static $blob1 48 b0
  set_static $blob1 56 b0
  set_static $blob1 64 b10
  call @5 b0
  return
fn @$sweep_block (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: bool binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = raw_load8 b6 0
  b8: int = raw_load64 b0 0
  b9: int = const 2
  b10: int = const 1
  b11: bool = equal b7 b9
  branch b11 %0 %1
  label %0
  raw_store8 b6 0 b10
  return b8
  label %1
  b11: bool = equal b7 b10
  branch b11 %2 %3
  label %2
  call @12 b0 b8
  return b8
  label %3
  return b8
fn @$sweep () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: int binding
  b0: int = get_static $blob1 24
  b1: int = get_static $blob1 32
  b2: int = copy b0
  label %0
  b3: bool = less_than b2 b1
  branch b3 %1 %2
  label %1
  b4: int = call @13 b2
  b2: int = add b2 b4
  jump %0
  label %2
  b5: int = const 
  set_static $blob1 56 b5
  set_static $blob1 64 b5
  return
fn @$collect () -> void
  b0: int binding
  b1: int binding
  b2: bool binding
  call @10
  b0: int = const 0
  label %0
  b1: int = call @11
  b2: bool = equal b1 b0
  branch b2 %1 %0
  label %1
  call @14
  return
fn @$alloc (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: bool binding
  b1: int = const 0
  b2: int = get_static $blob1 32
  b3: bool = less_than b2 b1
  branch b3 %0 %1
  label %0
  call @2
  label %1
  b4: int = call @3 b0
  b5: int = call @6 b4
  b6: bool = equal b5 b1
  branch b6 %2 %7
  label %2
  b7: int = call @7 b4
  b8: bool = equal b7 b1
  branch b8 %3 %6
  label %3
  call @15
  b5: int = call @6 b4
  b6: bool = equal b5 b1
  branch b6 %4 %7
  label %4
  b7: int = call @7 b4
  b8: bool = equal b7 b1
  branch b8 %5 %6
  label %5
  fail_oom
  label %6
  b5: int = copy b7
  label %7
  return b5
fn @$string_length (p0: ref) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b1: int = const 0
  b2: int = copy b0
  b3: int = copy b1
  b4: bool = equal b2 b1
  branch b4 %0 %1
  label %0
  return b3
  label %1
  b3: int = raw_load64 b0 8
  return b3
fn @$copy_bytes (p0: int, p1: int, p2: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b3: int = const 0
  b4: int = const 1
  label %0
  b5: bool = less_than b3 b2
  branch b5 %2 %1
  label %2
  b6: int = add b1 b3
  b7: int = raw_load8 b6 0
  b8: int = add b0 b3
  raw_store8 b8 0 b7
  b3: int = add b3 b4
  jump %0
  label %1
  return
fn @$string_equal (p0: ref, p1: ref) -> bool
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: int binding
  b11: bool binding
  b12: bool binding
  b13: bool binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: int binding
  b18: int binding
  b19: bool binding
  b2: int = const 0
  b3: int = const 1
  b4: int = copy b0
  b5: int = copy b1
  b6: int = copy b2
  b7: int = copy b2
  b8: bool = equal b4 b2
  branch b8 %0 %1
  label %1
  b6: int = raw_load64 b0 8
  b7: int = raw_load64 b0 16
  jump %2
  label %0
  label %2
  b9: int = copy b2
  b10: int = copy b2
  b11: bool = equal b5 b2
  branch b11 %3 %4
  label %4
  b9: int = raw_load64 b1 8
  b10: int = raw_load64 b1 16
  jump %5
  label %3
  label %5
  b12: bool = equal b6 b9
  branch b12 %6 %7
  label %7
  b13: bool = const false
  return b13
  label %6
  b14: int = const 0
  label %8
  b12: bool = less_than b14 b6
  branch b12 %9 %10
  label %9
  b15: int = add b7 b14
  b16: int = raw_load8 b15 0
  b17: int = add b10 b14
  b18: int = raw_load8 b17 0
  b19: bool = equal b16 b18
  branch b19 %11 %7
  label %11
  b14: int = add b14 b3
  jump %8
  label %10
  b13: bool = const true
  return b13
fn @$string_concat (p0: ref, p1: ref) -> ref
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: bool binding
  b11: int binding
  b12: int binding
  b13: int binding
  b14: int binding
  b15: ref binding
  b16: int binding
  b17: int binding
  b18: int binding
  b2: int = const 0
  b3: int = copy b0
  b4: int = copy b2
  b5: int = copy b2
  b6: bool = equal b3 b2
  branch b6 %0 %1
  label %1
  b4: int = raw_load64 b0 8
  b5: int = raw_load64 b0 16
  jump %2
  label %0
  label %2
  b7: int = copy b1
  b8: int = copy b2
  b9: int = copy b2
  b10: bool = equal b7 b2
  branch b10 %3 %4
  label %4
  b8: int = raw_load64 b1 8
  b9: int = raw_load64 b1 16
  jump %5
  label %3
  label %5
  b11: int = add b4 b8
  b12: int = const 16
  b13: int = add b11 b12
  b14: int = call @16 b13
  b15: ref = copy b14
  b16: int = const 24
  b17: int = add b15 b16
  raw_store64 b15 8 b11
  raw_store64 b15 16 b17
  call @18 b17 b5 b4
  b18: int = add b17 b4
  call @18 b18 b9 b8
  return b15
fn @$write_byte_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  set_static $blob24 24 b1
  b2: ref = data $blob24
  b3: ref = raw_load64 b2 16
  b4: int = const 1
  b5: int = raw_syscall syscall b4 b0 b3 b4
  return
fn @$write_blob_fd (p0: int, p1: ref) -> void
  b0: int binding
  b1: ref binding
  b2: int binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b2: int = raw_load64 b1 8
  b3: ref = raw_load64 b1 16
  b4: int = const 1
  b5: int = raw_syscall syscall b4 b0 b3 b2
  return
fn @$write_digits_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b2: int = const 10
  b3: int = whole_divide b1 b2
  b4: int = whole_remainder b1 b2
  b5: int = const 0
  b6: bool = equal b3 b5
  branch b6 %0 %1
  label %1
  call @23 b0 b3
  label %0
  b7: int = negate b4
  b8: int = const 48
  b9: int = add b7 b8
  call @21 b0 b9
  return
fn @$write_int_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b2: int = const 0
  b3: bool = equal b1 b2
  branch b3 %0 %1
  label %0
  b4: int = const 48
  call @21 b0 b4
  return
  label %1
  b5: bool = less_than b1 b2
  branch b5 %2 %3
  label %2
  b6: int = const 45
  call @21 b0 b6
  call @23 b0 b1
  return
  label %3
  b7: int = negate b1
  call @23 b0 b7
  return
fn @$fail_blob (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b6: int binding
  b1: int = const 2
  b2: ref = data $blob3
  call @22 b1 b2
  call @22 b1 b0
  b3: ref = data $blob4
  call @22 b1 b3
  b4: int = const 60
  b5: int = const 1
  b6: int = raw_syscall syscall b4 b5
  return
fn @$fail_bounds (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: ref binding
  b4: ref binding
  b5: ref binding
  b6: int binding
  b7: int binding
  b8: int binding
  b2: int = const 2
  b3: ref = data $blob5
  call @22 b2 b3
  call @24 b2 b0
  b4: ref = data $blob6
  call @22 b2 b4
  call @24 b2 b1
  b5: ref = data $blob4
  call @22 b2 b5
  b6: int = const 60
  b7: int = const 1
  b8: int = raw_syscall syscall b6 b7
  return
fn @$fail_nul (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b6: int binding
  b1: int = const 2
  b2: ref = data $blob13
  call @22 b1 b2
  call @24 b1 b0
  b3: ref = data $blob4
  call @22 b1 b3
  b4: int = const 60
  b5: int = const 1
  b6: int = raw_syscall syscall b4 b5
  return
fn @$fail_string_index (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: ref binding
  b4: ref binding
  b5: ref binding
  b6: int binding
  b7: int binding
  b8: int binding
  b2: int = const 2
  b3: ref = data $blob16
  call @22 b2 b3
  call @24 b2 b0
  b4: ref = data $blob6
  call @22 b2 b4
  call @24 b2 b1
  b5: ref = data $blob4
  call @22 b2 b5
  b6: int = const 60
  b7: int = const 1
  b8: int = raw_syscall syscall b6 b7
  return
fn @$fail_substring (p0: int, p1: int, p2: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: ref binding
  b5: ref binding
  b6: ref binding
  b7: ref binding
  b8: int binding
  b9: int binding
  b10: int binding
  b3: int = const 2
  b4: ref = data $blob17
  call @22 b3 b4
  call @24 b3 b0
  b5: ref = data $blob20
  call @22 b3 b5
  call @24 b3 b1
  b6: ref = data $blob6
  call @22 b3 b6
  call @24 b3 b2
  b7: ref = data $blob4
  call @22 b3 b7
  b8: int = const 60
  b9: int = const 1
  b10: int = raw_syscall syscall b8 b9
  return
fn @$fail_shift (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: ref binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = const 2
  b2: ref = data $blob18
  call @22 b1 b2
  call @24 b1 b0
  b3: ref = data $blob19
  call @22 b1 b3
  b4: ref = data $blob4
  call @22 b1 b4
  b5: int = const 60
  b6: int = const 1
  b7: int = raw_syscall syscall b5 b6
  return
fn @$zero_bytes (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 0
  label %0
  b5: bool = less_than b2 b1
  branch b5 %1 %2
  label %1
  b6: int = add b0 b2
  raw_store8 b6 0 b4
  b2: int = add b2 b3
  jump %0
  label %2
  return
fn @$array_new (p0: int, p1: int) -> ref
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: ref binding
  b8: bool binding
  b9: ref binding
  b10: bool binding
  b11: ref binding
  b12: int binding
  b13: bool binding
  b14: ref binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: int binding
  b19: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 32
  b5: int = const 8
  b6: int = const 9223372036854775807
  b7: ref = copy b2
  b8: bool = less_than b0 b2
  branch b8 %0 %1
  label %0
  b9: ref = data $blob7
  call @25 b9
  return b7
  label %1
  b10: bool = less_than b1 b3
  branch b10 %2 %3
  label %2
  b11: ref = data $blob8
  call @25 b11
  return b7
  label %3
  b12: int = whole_divide b6 b1
  b13: bool = less_than b12 b0
  branch b13 %4 %5
  label %4
  b14: ref = data $blob9
  call @25 b14
  return b7
  label %5
  b15: int = call @16 b4
  raw_store64 b15 8 b0
  raw_store64 b15 16 b1
  raw_store64 b15 24 b2
  raw_store64 b15 32 b0
  b7: ref = copy b15
  b16: int = multiply b0 b1
  b17: bool = equal b16 b2
  branch b17 %6 %7
  label %6
  b16: int = const 1
  label %7
  b18: int = call @16 b16
  raw_store64 b15 24 b18
  b19: int = add b18 b5
  call @31 b19 b16
  return b7
fn @$array_length (p0: ref) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: ref binding
  b5: int binding
  b1: int = const 0
  b2: int = copy b0
  b3: bool = equal b2 b1
  branch b3 %0 %1
  label %0
  b4: ref = data $blob11
  call @25 b4
  return b1
  label %1
  b5: int = raw_load64 b0 8
  return b5
fn @$array_push (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: bool binding
  b8: ref binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: bool binding
  b13: int binding
  b14: bool binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: ref binding
  b19: int binding
  b20: int binding
  b21: int binding
  b22: int binding
  b23: int binding
  b24: int binding
  b25: int binding
  b26: int binding
  b27: int binding
  b28: int binding
  b29: bool binding
  b30: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 9223372036854775807
  b5: int = const 8
  b6: int = copy b0
  b7: bool = equal b6 b2
  branch b7 %0 %1
  label %0
  b8: ref = data $blob12
  call @25 b8
  return b2
  label %1
  b9: int = raw_load64 b0 8
  b10: int = raw_load64 b0 16
  b11: int = raw_load64 b0 32
  b12: bool = equal b9 b11
  branch b12 %2 %8
  label %2
  b13: int = copy b11
  b14: bool = less_than b11 b3
  branch b14 %3 %4
  label %3
  b13: int = copy b3
  jump %5
  label %4
  b15: int = add b11 b11
  b13: int = copy b15
  label %5
  b16: int = whole_divide b4 b10
  b17: bool = less_than b16 b13
  branch b17 %6 %7
  label %6
  b18: ref = data $blob9
  call @25 b18
  return b2
  label %7
  b19: int = multiply b13 b10
  b20: int = call @16 b19
  b21: int = multiply b9 b10
  b22: int = raw_load64 b0 24
  b23: int = add b20 b5
  b24: int = add b22 b5
  call @18 b23 b24 b21
  raw_store64 b0 24 b20
  raw_store64 b0 32 b13
  label %8
  b25: int = raw_load64 b0 24
  b26: int = add b25 b5
  b27: int = multiply b9 b10
  b28: int = add b26 b27
  b29: bool = equal b10 b3
  branch b29 %9 %10
  label %9
  raw_store8 b28 0 b1
  jump %11
  label %10
  raw_store64 b28 0 b1
  label %11
  b30: int = add b9 b3
  raw_store64 b0 8 b30
  return b30
fn @$array_at (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: bool binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: int binding
  b14: int binding
  b2: int = const 0
  b3: int = const 8
  b4: int = copy b0
  b5: bool = equal b4 b2
  branch b5 %0 %1
  label %0
  b6: ref = data $blob10
  call @25 b6
  return b2
  label %1
  b7: int = raw_load64 b0 8
  b8: bool = less_than b1 b2
  branch b8 %2 %3
  label %3
  b9: bool = less_than b1 b7
  branch b9 %4 %2
  label %2
  call @26 b1 b7
  return b2
  label %4
  b10: int = raw_load64 b0 16
  b11: int = raw_load64 b0 24
  b12: int = multiply b1 b10
  b13: int = add b11 b3
  b14: int = add b13 b12
  return b14
fn @$string_to_bytes (p0: ref) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: ref binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = raw_load64 b0 8
  b2: int = raw_load64 b0 16
  b3: int = const 1
  b4: ref = call @32 b1 b3
  b5: int = raw_load64 b4 24
  b6: int = const 8
  b7: int = add b5 b6
  call @18 b7 b2 b1
  return b4
fn @$bytes_to_string (p0: ref) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: ref binding
  b10: int binding
  b11: int binding
  b12: bool binding
  b13: ref binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: int binding
  b19: int binding
  b20: bool binding
  b21: int binding
  b22: int binding
  b23: int binding
  b2: int = const 24
  b1: int = const 0
  b3: int = const 1
  b4: int = const 8
  b5: int = const 16
  b6: ref = copy b1
  b7: int = copy b0
  b8: bool = equal b7 b1
  branch b8 %0 %1
  label %0
  b9: ref = data $blob14
  call @25 b9
  return b6
  label %1
  b10: int = raw_load64 b0 8
  b11: int = raw_load64 b0 16
  b12: bool = equal b11 b3
  branch b12 %2 %3
  label %3
  b13: ref = data $blob15
  call @25 b13
  return b6
  label %2
  b14: int = raw_load64 b0 24
  b15: int = add b14 b4
  b16: int = const 0
  label %4
  b17: bool = less_than b16 b10
  branch b17 %5 %6
  label %5
  b19: int = add b15 b16
  b18: int = raw_load8 b19 0
  b20: bool = equal b18 b1
  branch b20 %7 %8
  label %8
  b16: int = add b16 b3
  jump %4
  label %7
  call @27 b16
  return b6
  label %6
  b21: int = add b10 b5
  b22: int = call @16 b21
  raw_store64 b22 8 b10
  b23: int = add b22 b2
  raw_store64 b22 16 b23
  call @18 b23 b15 b10
  b6: ref = copy b22
  return b6
fn @$string_at (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b5: int binding
  b6: bool binding
  b7: bool binding
  b8: int binding
  b9: int binding
  b10: int binding
  b2: int = const 0
  b3: int = copy b0
  b4: bool = equal b3 b2
  b5: int = const 0
  branch b4 %1 %0
  label %0
  b5: int = raw_load64 b0 8
  label %1
  b6: bool = less_than b1 b2
  branch b6 %2 %3
  label %3
  b7: bool = less_than b1 b5
  branch b7 %4 %2
  label %2
  call @28 b1 b5
  return b2
  label %4
  b8: int = raw_load64 b0 16
  b9: int = add b8 b1
  b10: int = raw_load8 b9 0
  return b10
fn @$substring (p0: ref, p1: int, p2: int) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: bool binding
  b11: bool binding
  b12: bool binding
  b13: int binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: int binding
  b18: int binding
  b4: int = const 24
  b3: int = const 0
  b5: int = const 16
  b6: ref = copy b3
  b7: int = copy b0
  b8: bool = equal b7 b3
  b9: int = const 0
  branch b8 %1 %0
  label %0
  b9: int = raw_load64 b0 8
  label %1
  b10: bool = less_than b1 b3
  branch b10 %2 %3
  label %3
  b11: bool = less_than b2 b1
  branch b11 %2 %4
  label %4
  b12: bool = less_than b9 b2
  branch b12 %2 %5
  label %2
  call @29 b1 b2 b9
  return b6
  label %5
  b13: int = subtract b2 b1
  b14: int = add b13 b5
  b15: int = call @16 b14
  raw_store64 b15 8 b13
  b16: int = add b15 b4
  raw_store64 b15 16 b16
  b17: int = raw_load64 b0 16
  b18: int = add b17 b1
  call @18 b16 b18 b13
  b6: ref = copy b15
  return b6
fn @$print_string_raw (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b4: int binding
  b1: int = raw_load64 b0 8
  b2: ref = raw_load64 b0 16
  b3: int = const 1
  b4: int = raw_syscall syscall b3 b3 b2 b1
  return
fn @$print (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b4: int binding
  b5: ref binding
  b6: ref binding
  b7: int binding
  b8: int binding
  b1: int = raw_load64 b0 8
  b2: ref = raw_load64 b0 16
  b3: int = const 1
  b4: int = raw_syscall syscall b3 b3 b2 b1
  b5: ref = data $blob25
  b6: ref = raw_load64 b5 16
  b7: int = const 1
  b8: int = raw_syscall syscall b7 b7 b6 b7
  return
fn @$print_newline () -> void
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b0: ref = data $blob25
  b1: ref = raw_load64 b0 16
  b2: int = const 1
  b3: int = raw_syscall syscall b2 b2 b1 b2
  return

`

// The Phase 10.5 float render, pinned: float literals bind their IEEE 754
// bit patterns as `constf`, the SSE2 forms carry the `f` suffix, and the
// branch shapes stay identical to the scalar slices.
const wantQuartersScene = `scene
data $blob0: 8 bytes 32 32 32 32 32 32 32 32
data $blob1: 48 bytes 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255
data $blob2: 16 bytes 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255
data $blob3: 6 bytes 112 117 114 114 58 32
data $blob4: 1 bytes 10
data $blob5: 18 bytes 112 117 114 114 58 32 97 114 114 97 121 32 105 110 100 101 120 32
data $blob6: 28 bytes 32 105 115 32 111 117 116 32 111 102 32 114 97 110 103 101 32 102 111 114 32 108 101 110 103 116 104 32
data $blob7: 31 bytes 97 114 114 97 121 32 108 101 110 103 116 104 32 99 97 110 110 111 116 32 98 101 32 110 101 103 97 116 105 118 101
data $blob8: 35 bytes 97 114 114 97 121 32 101 108 101 109 101 110 116 32 115 105 122 101 32 109 117 115 116 32 98 101 32 112 111 115 105 116 105 118 101
data $blob9: 29 bytes 97 114 114 97 121 32 97 108 108 111 99 97 116 105 111 110 32 105 115 32 116 111 111 32 108 97 114 103 101
data $blob10: 38 bytes 97 114 114 97 121 32 97 99 99 101 115 115 32 111 110 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob11: 32 bytes 108 101 110 103 116 104 32 111 102 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob12: 32 bytes 97 112 112 101 110 100 32 116 111 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob13: 46 bytes 112 117 114 114 58 32 98 121 116 101 32 97 114 114 97 121 32 99 111 110 116 97 105 110 115 32 97 32 78 85 76 32 98 121 116 101 32 97 116 32 105 110 100 101 120 32
data $blob14: 18 bytes 98 121 116 101 32 97 114 114 97 121 32 105 115 32 110 117 108 108
data $blob15: 32 bytes 98 121 116 101 32 97 114 114 97 121 32 101 108 101 109 101 110 116 32 115 105 122 101 32 105 115 32 110 111 116 32 49
data $blob16: 19 bytes 112 117 114 114 58 32 115 116 114 105 110 103 32 105 110 100 101 120 32
data $blob17: 22 bytes 112 117 114 114 58 32 115 117 98 115 116 114 105 110 103 32 114 97 110 103 101 32
data $blob18: 18 bytes 112 117 114 114 58 32 115 104 105 102 116 32 99 111 117 110 116 32
data $blob19: 17 bytes 32 105 115 32 111 117 116 115 105 100 101 32 48 46 46 54 51
data $blob20: 2 bytes 46 46
data $blob21: 1 bytes 32
data $blob22: 4 bytes 116 114 117 101
data $blob23: 5 bytes 102 97 108 115 101
data $blob24: 1 bytes 48
data $blob25: 1 bytes 10
data $blob26: 1 bytes 32
data $blob27: 8 bytes 113 117 97 114 116 101 114 115
entry 0 @main
fn @main () -> void
  b0: float64 binding
  b1: float64 binding
  b2: float64 binding
  b3: float64 binding
  b4: bool binding
  b5: ref binding
  b0: float = constf 4602678819172646912
  b1: float = constf 4598175219545276416
  b2: float = addf b0 b1
  b3: float = constf 4604930618986332160
  b4: bool = equalf b2 b3
  branch b4 %0 %1
  label %0
  b5: ref = data $blob27
  call @41 b5
  label %1
fn @$mmap (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b1: int = const 0
  b2: int = const 3
  b3: int = const 34
  b4: int = const 9
  b5: int = raw_syscall syscall b4 b1 b0 b2 b3
  b6: bool = less_than b5 b1
  branch b6 %0 %1
  label %0
  fail_oom
  label %1
  return b5
fn @$map () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b0: int = const 536870912
  b1: int = const 67108872
  b2: int = call @1 b0
  b3: int = call @1 b1
  b4: int = get_static $blob0 24
  b5: int = const 
  b6: int = add b2 b0
  set_static $blob1 24 b2
  set_static $blob1 32 b2
  set_static $blob1 40 b6
  set_static $blob1 48 b5
  set_static $blob1 56 b5
  set_static $blob1 64 b5
  set_static $blob2 24 b3
  set_static $blob2 32 b4
  return
fn @$pad8 (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: bool binding
  b1: int = const 15
  b2: int = const 8
  b3: int = const 16
  b4: int = add b0 b1
  b5: int = whole_divide b4 b2
  b6: int = multiply b5 b2
  b7: bool = less_than b6 b3
  branch b7 %0 %1
  label %0
  b6: int = copy b3
  label %1
  return b6
fn @$byte_set1 (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = const 1
  raw_store8 b6 0 b7
  return
fn @$byte_set0 (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = const 0
  raw_store8 b6 0 b7
  return
fn @$search (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: bool binding
  b11: int binding
  b12: bool binding
  b13: int binding
  b14: int binding
  b1: int = const 0
  b2: int = get_static $blob1 48
  b3: int = copy b1
  b4: int = copy b2
  b5: int = const 16
  label %0
  b6: bool = less_than b4 b1
  branch b6 %9 %1
  label %1
  b7: int = raw_load64 b4 0
  b8: bool = less_than b7 b0
  branch b8 %2 %3
  label %2
  b3: int = copy b4
  b4: int = raw_load64 b4 8
  jump %0
  label %3
  b9: int = raw_load64 b4 8
  b10: bool = equal b3 b1
  branch b10 %4 %5
  label %4
  set_static $blob1 48 b9
  jump %6
  label %5
  raw_store64 b3 8 b9
  label %6
  b11: int = subtract b7 b0
  b12: bool = less_than b11 b5
  branch b12 %8 %7
  label %7
  b13: int = add b4 b0
  raw_store64 b13 0 b11
  b14: int = get_static $blob1 48
  raw_store64 b13 8 b14
  set_static $blob1 48 b13
  jump %8
  label %8
  raw_store64 b4 0 b0
  call @4 b4
  return b4
  label %9
  return b1
fn @$bump (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b1: int = const 0
  b2: int = get_static $blob1 32
  b3: int = get_static $blob1 40
  b4: int = add b2 b0
  b5: bool = less_than b3 b4
  branch b5 %0 %1
  label %1
  set_static $blob1 32 b4
  raw_store64 b2 0 b0
  call @4 b2
  return b2
  label %0
  return b1
fn @$mark_at (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: bool binding
  b14: int binding
  b1: int = const 0
  b2: int = get_static $blob1 24
  b3: bool = less_than b0 b2
  branch b3 %0 %1
  label %1
  b4: int = get_static $blob1 40
  b5: bool = less_than b4 b0
  branch b5 %0 %2
  label %2
  b6: int = const 8
  b7: int = subtract b0 b2
  b8: int = whole_divide b7 b6
  b9: int = get_static $blob2 24
  b10: int = add b9 b8
  b11: int = raw_load8 b10 0
  b12: int = const 1
  b13: bool = equal b11 b12
  branch b13 %3 %0
  label %3
  b14: int = const 2
  raw_store8 b10 0 b14
  b1: int = const 1
  label %0
  return b1
fn @$ledger_at (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b1: int = const 0
  b2: int = get_static $blob1 24
  b3: bool = less_than b0 b2
  branch b3 %0 %1
  label %1
  b4: int = get_static $blob1 40
  b5: bool = less_than b4 b0
  branch b5 %0 %2
  label %2
  b6: int = const 8
  b7: int = subtract b0 b2
  b8: int = whole_divide b7 b6
  b9: int = get_static $blob2 24
  b10: int = add b9 b8
  b1: int = raw_load8 b10 0
  label %0
  return b1
fn @$scan_roots () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b0: int = get_static $blob2 32
  b1: int = stack_pointer
  b2: int = const 7
  b3: int = const 8
  b4: int = subtract b1 b2
  b4: int = whole_divide b4 b3
  b4: int = multiply b4 b3
  label %0
  b5: bool = less_than b4 b0
  branch b5 %1 %2
  label %1
  b6: int = raw_load64 b4 0
  b7: int = call @8 b6
  b4: int = add b4 b3
  jump %0
  label %2
  return
fn @$scan_heap () -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: bool binding
  b14: int binding
  b15: int binding
  b16: bool binding
  b17: int binding
  b18: int binding
  b19: bool binding
  b0: int = const 0
  b1: int = const 2
  b2: int = const 8
  b3: int = get_static $blob1 24
  b4: int = get_static $blob1 32
  b5: int = copy b3
  b6: int = copy b0
  label %0
  b19: bool = less_than b5 b4
  branch b19 %1 %2
  label %1
  b7: int = raw_load64 b5 0
  b8: int = subtract b5 b3
  b9: int = whole_divide b8 b2
  b10: int = get_static $blob2 24
  b11: int = add b10 b9
  b12: int = raw_load8 b11 0
  b13: bool = equal b12 b1
  branch b13 %3 %4
  label %3
  b14: int = add b5 b2
  b15: int = add b5 b7
  label %5
  b16: bool = less_than b14 b15
  branch b16 %6 %4
  label %6
  b17: int = raw_load64 b14 0
  b18: int = call @8 b17
  b6: int = add b6 b18
  b14: int = add b14 b2
  jump %5
  label %4
  b5: int = add b5 b7
  jump %0
  label %2
  return b6
fn @$free_block (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b2: int = const 0
  b3: int = get_static $blob1 48
  b4: int = get_static $blob1 56
  b5: int = get_static $blob1 64
  b6: bool = equal b5 b0
  branch b6 %0 %1
  label %0
  b7: int = raw_load64 b4 0
  b8: int = add b7 b1
  raw_store64 b4 0 b8
  b9: int = add b0 b1
  set_static $blob1 64 b9
  call @5 b0
  return
  label %1
  b10: int = add b0 b1
  raw_store64 b0 8 b3
  set_static $blob1 48 b0
  set_static $blob1 56 b0
  set_static $blob1 64 b10
  call @5 b0
  return
fn @$sweep_block (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: bool binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = raw_load8 b6 0
  b8: int = raw_load64 b0 0
  b9: int = const 2
  b10: int = const 1
  b11: bool = equal b7 b9
  branch b11 %0 %1
  label %0
  raw_store8 b6 0 b10
  return b8
  label %1
  b11: bool = equal b7 b10
  branch b11 %2 %3
  label %2
  call @12 b0 b8
  return b8
  label %3
  return b8
fn @$sweep () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: int binding
  b0: int = get_static $blob1 24
  b1: int = get_static $blob1 32
  b2: int = copy b0
  label %0
  b3: bool = less_than b2 b1
  branch b3 %1 %2
  label %1
  b4: int = call @13 b2
  b2: int = add b2 b4
  jump %0
  label %2
  b5: int = const 
  set_static $blob1 56 b5
  set_static $blob1 64 b5
  return
fn @$collect () -> void
  b0: int binding
  b1: int binding
  b2: bool binding
  call @10
  b0: int = const 0
  label %0
  b1: int = call @11
  b2: bool = equal b1 b0
  branch b2 %1 %0
  label %1
  call @14
  return
fn @$alloc (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: bool binding
  b1: int = const 0
  b2: int = get_static $blob1 32
  b3: bool = less_than b2 b1
  branch b3 %0 %1
  label %0
  call @2
  label %1
  b4: int = call @3 b0
  b5: int = call @6 b4
  b6: bool = equal b5 b1
  branch b6 %2 %7
  label %2
  b7: int = call @7 b4
  b8: bool = equal b7 b1
  branch b8 %3 %6
  label %3
  call @15
  b5: int = call @6 b4
  b6: bool = equal b5 b1
  branch b6 %4 %7
  label %4
  b7: int = call @7 b4
  b8: bool = equal b7 b1
  branch b8 %5 %6
  label %5
  fail_oom
  label %6
  b5: int = copy b7
  label %7
  return b5
fn @$string_length (p0: ref) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b1: int = const 0
  b2: int = copy b0
  b3: int = copy b1
  b4: bool = equal b2 b1
  branch b4 %0 %1
  label %0
  return b3
  label %1
  b3: int = raw_load64 b0 8
  return b3
fn @$copy_bytes (p0: int, p1: int, p2: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b3: int = const 0
  b4: int = const 1
  label %0
  b5: bool = less_than b3 b2
  branch b5 %2 %1
  label %2
  b6: int = add b1 b3
  b7: int = raw_load8 b6 0
  b8: int = add b0 b3
  raw_store8 b8 0 b7
  b3: int = add b3 b4
  jump %0
  label %1
  return
fn @$string_equal (p0: ref, p1: ref) -> bool
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: int binding
  b11: bool binding
  b12: bool binding
  b13: bool binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: int binding
  b18: int binding
  b19: bool binding
  b2: int = const 0
  b3: int = const 1
  b4: int = copy b0
  b5: int = copy b1
  b6: int = copy b2
  b7: int = copy b2
  b8: bool = equal b4 b2
  branch b8 %0 %1
  label %1
  b6: int = raw_load64 b0 8
  b7: int = raw_load64 b0 16
  jump %2
  label %0
  label %2
  b9: int = copy b2
  b10: int = copy b2
  b11: bool = equal b5 b2
  branch b11 %3 %4
  label %4
  b9: int = raw_load64 b1 8
  b10: int = raw_load64 b1 16
  jump %5
  label %3
  label %5
  b12: bool = equal b6 b9
  branch b12 %6 %7
  label %7
  b13: bool = const false
  return b13
  label %6
  b14: int = const 0
  label %8
  b12: bool = less_than b14 b6
  branch b12 %9 %10
  label %9
  b15: int = add b7 b14
  b16: int = raw_load8 b15 0
  b17: int = add b10 b14
  b18: int = raw_load8 b17 0
  b19: bool = equal b16 b18
  branch b19 %11 %7
  label %11
  b14: int = add b14 b3
  jump %8
  label %10
  b13: bool = const true
  return b13
fn @$string_concat (p0: ref, p1: ref) -> ref
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: bool binding
  b11: int binding
  b12: int binding
  b13: int binding
  b14: int binding
  b15: ref binding
  b16: int binding
  b17: int binding
  b18: int binding
  b2: int = const 0
  b3: int = copy b0
  b4: int = copy b2
  b5: int = copy b2
  b6: bool = equal b3 b2
  branch b6 %0 %1
  label %1
  b4: int = raw_load64 b0 8
  b5: int = raw_load64 b0 16
  jump %2
  label %0
  label %2
  b7: int = copy b1
  b8: int = copy b2
  b9: int = copy b2
  b10: bool = equal b7 b2
  branch b10 %3 %4
  label %4
  b8: int = raw_load64 b1 8
  b9: int = raw_load64 b1 16
  jump %5
  label %3
  label %5
  b11: int = add b4 b8
  b12: int = const 16
  b13: int = add b11 b12
  b14: int = call @16 b13
  b15: ref = copy b14
  b16: int = const 24
  b17: int = add b15 b16
  raw_store64 b15 8 b11
  raw_store64 b15 16 b17
  call @18 b17 b5 b4
  b18: int = add b17 b4
  call @18 b18 b9 b8
  return b15
fn @$write_byte_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  set_static $blob24 24 b1
  b2: ref = data $blob24
  b3: ref = raw_load64 b2 16
  b4: int = const 1
  b5: int = raw_syscall syscall b4 b0 b3 b4
  return
fn @$write_blob_fd (p0: int, p1: ref) -> void
  b0: int binding
  b1: ref binding
  b2: int binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b2: int = raw_load64 b1 8
  b3: ref = raw_load64 b1 16
  b4: int = const 1
  b5: int = raw_syscall syscall b4 b0 b3 b2
  return
fn @$write_digits_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b2: int = const 10
  b3: int = whole_divide b1 b2
  b4: int = whole_remainder b1 b2
  b5: int = const 0
  b6: bool = equal b3 b5
  branch b6 %0 %1
  label %1
  call @23 b0 b3
  label %0
  b7: int = negate b4
  b8: int = const 48
  b9: int = add b7 b8
  call @21 b0 b9
  return
fn @$write_int_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b2: int = const 0
  b3: bool = equal b1 b2
  branch b3 %0 %1
  label %0
  b4: int = const 48
  call @21 b0 b4
  return
  label %1
  b5: bool = less_than b1 b2
  branch b5 %2 %3
  label %2
  b6: int = const 45
  call @21 b0 b6
  call @23 b0 b1
  return
  label %3
  b7: int = negate b1
  call @23 b0 b7
  return
fn @$fail_blob (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b6: int binding
  b1: int = const 2
  b2: ref = data $blob3
  call @22 b1 b2
  call @22 b1 b0
  b3: ref = data $blob4
  call @22 b1 b3
  b4: int = const 60
  b5: int = const 1
  b6: int = raw_syscall syscall b4 b5
  return
fn @$fail_bounds (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: ref binding
  b4: ref binding
  b5: ref binding
  b6: int binding
  b7: int binding
  b8: int binding
  b2: int = const 2
  b3: ref = data $blob5
  call @22 b2 b3
  call @24 b2 b0
  b4: ref = data $blob6
  call @22 b2 b4
  call @24 b2 b1
  b5: ref = data $blob4
  call @22 b2 b5
  b6: int = const 60
  b7: int = const 1
  b8: int = raw_syscall syscall b6 b7
  return
fn @$fail_nul (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b6: int binding
  b1: int = const 2
  b2: ref = data $blob13
  call @22 b1 b2
  call @24 b1 b0
  b3: ref = data $blob4
  call @22 b1 b3
  b4: int = const 60
  b5: int = const 1
  b6: int = raw_syscall syscall b4 b5
  return
fn @$fail_string_index (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: ref binding
  b4: ref binding
  b5: ref binding
  b6: int binding
  b7: int binding
  b8: int binding
  b2: int = const 2
  b3: ref = data $blob16
  call @22 b2 b3
  call @24 b2 b0
  b4: ref = data $blob6
  call @22 b2 b4
  call @24 b2 b1
  b5: ref = data $blob4
  call @22 b2 b5
  b6: int = const 60
  b7: int = const 1
  b8: int = raw_syscall syscall b6 b7
  return
fn @$fail_substring (p0: int, p1: int, p2: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: ref binding
  b5: ref binding
  b6: ref binding
  b7: ref binding
  b8: int binding
  b9: int binding
  b10: int binding
  b3: int = const 2
  b4: ref = data $blob17
  call @22 b3 b4
  call @24 b3 b0
  b5: ref = data $blob20
  call @22 b3 b5
  call @24 b3 b1
  b6: ref = data $blob6
  call @22 b3 b6
  call @24 b3 b2
  b7: ref = data $blob4
  call @22 b3 b7
  b8: int = const 60
  b9: int = const 1
  b10: int = raw_syscall syscall b8 b9
  return
fn @$fail_shift (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: ref binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = const 2
  b2: ref = data $blob18
  call @22 b1 b2
  call @24 b1 b0
  b3: ref = data $blob19
  call @22 b1 b3
  b4: ref = data $blob4
  call @22 b1 b4
  b5: int = const 60
  b6: int = const 1
  b7: int = raw_syscall syscall b5 b6
  return
fn @$zero_bytes (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 0
  label %0
  b5: bool = less_than b2 b1
  branch b5 %1 %2
  label %1
  b6: int = add b0 b2
  raw_store8 b6 0 b4
  b2: int = add b2 b3
  jump %0
  label %2
  return
fn @$array_new (p0: int, p1: int) -> ref
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: ref binding
  b8: bool binding
  b9: ref binding
  b10: bool binding
  b11: ref binding
  b12: int binding
  b13: bool binding
  b14: ref binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: int binding
  b19: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 32
  b5: int = const 8
  b6: int = const 9223372036854775807
  b7: ref = copy b2
  b8: bool = less_than b0 b2
  branch b8 %0 %1
  label %0
  b9: ref = data $blob7
  call @25 b9
  return b7
  label %1
  b10: bool = less_than b1 b3
  branch b10 %2 %3
  label %2
  b11: ref = data $blob8
  call @25 b11
  return b7
  label %3
  b12: int = whole_divide b6 b1
  b13: bool = less_than b12 b0
  branch b13 %4 %5
  label %4
  b14: ref = data $blob9
  call @25 b14
  return b7
  label %5
  b15: int = call @16 b4
  raw_store64 b15 8 b0
  raw_store64 b15 16 b1
  raw_store64 b15 24 b2
  raw_store64 b15 32 b0
  b7: ref = copy b15
  b16: int = multiply b0 b1
  b17: bool = equal b16 b2
  branch b17 %6 %7
  label %6
  b16: int = const 1
  label %7
  b18: int = call @16 b16
  raw_store64 b15 24 b18
  b19: int = add b18 b5
  call @31 b19 b16
  return b7
fn @$array_length (p0: ref) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: ref binding
  b5: int binding
  b1: int = const 0
  b2: int = copy b0
  b3: bool = equal b2 b1
  branch b3 %0 %1
  label %0
  b4: ref = data $blob11
  call @25 b4
  return b1
  label %1
  b5: int = raw_load64 b0 8
  return b5
fn @$array_push (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: bool binding
  b8: ref binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: bool binding
  b13: int binding
  b14: bool binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: ref binding
  b19: int binding
  b20: int binding
  b21: int binding
  b22: int binding
  b23: int binding
  b24: int binding
  b25: int binding
  b26: int binding
  b27: int binding
  b28: int binding
  b29: bool binding
  b30: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 9223372036854775807
  b5: int = const 8
  b6: int = copy b0
  b7: bool = equal b6 b2
  branch b7 %0 %1
  label %0
  b8: ref = data $blob12
  call @25 b8
  return b2
  label %1
  b9: int = raw_load64 b0 8
  b10: int = raw_load64 b0 16
  b11: int = raw_load64 b0 32
  b12: bool = equal b9 b11
  branch b12 %2 %8
  label %2
  b13: int = copy b11
  b14: bool = less_than b11 b3
  branch b14 %3 %4
  label %3
  b13: int = copy b3
  jump %5
  label %4
  b15: int = add b11 b11
  b13: int = copy b15
  label %5
  b16: int = whole_divide b4 b10
  b17: bool = less_than b16 b13
  branch b17 %6 %7
  label %6
  b18: ref = data $blob9
  call @25 b18
  return b2
  label %7
  b19: int = multiply b13 b10
  b20: int = call @16 b19
  b21: int = multiply b9 b10
  b22: int = raw_load64 b0 24
  b23: int = add b20 b5
  b24: int = add b22 b5
  call @18 b23 b24 b21
  raw_store64 b0 24 b20
  raw_store64 b0 32 b13
  label %8
  b25: int = raw_load64 b0 24
  b26: int = add b25 b5
  b27: int = multiply b9 b10
  b28: int = add b26 b27
  b29: bool = equal b10 b3
  branch b29 %9 %10
  label %9
  raw_store8 b28 0 b1
  jump %11
  label %10
  raw_store64 b28 0 b1
  label %11
  b30: int = add b9 b3
  raw_store64 b0 8 b30
  return b30
fn @$array_at (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: bool binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: int binding
  b14: int binding
  b2: int = const 0
  b3: int = const 8
  b4: int = copy b0
  b5: bool = equal b4 b2
  branch b5 %0 %1
  label %0
  b6: ref = data $blob10
  call @25 b6
  return b2
  label %1
  b7: int = raw_load64 b0 8
  b8: bool = less_than b1 b2
  branch b8 %2 %3
  label %3
  b9: bool = less_than b1 b7
  branch b9 %4 %2
  label %2
  call @26 b1 b7
  return b2
  label %4
  b10: int = raw_load64 b0 16
  b11: int = raw_load64 b0 24
  b12: int = multiply b1 b10
  b13: int = add b11 b3
  b14: int = add b13 b12
  return b14
fn @$string_to_bytes (p0: ref) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: ref binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = raw_load64 b0 8
  b2: int = raw_load64 b0 16
  b3: int = const 1
  b4: ref = call @32 b1 b3
  b5: int = raw_load64 b4 24
  b6: int = const 8
  b7: int = add b5 b6
  call @18 b7 b2 b1
  return b4
fn @$bytes_to_string (p0: ref) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: ref binding
  b10: int binding
  b11: int binding
  b12: bool binding
  b13: ref binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: int binding
  b19: int binding
  b20: bool binding
  b21: int binding
  b22: int binding
  b23: int binding
  b2: int = const 24
  b1: int = const 0
  b3: int = const 1
  b4: int = const 8
  b5: int = const 16
  b6: ref = copy b1
  b7: int = copy b0
  b8: bool = equal b7 b1
  branch b8 %0 %1
  label %0
  b9: ref = data $blob14
  call @25 b9
  return b6
  label %1
  b10: int = raw_load64 b0 8
  b11: int = raw_load64 b0 16
  b12: bool = equal b11 b3
  branch b12 %2 %3
  label %3
  b13: ref = data $blob15
  call @25 b13
  return b6
  label %2
  b14: int = raw_load64 b0 24
  b15: int = add b14 b4
  b16: int = const 0
  label %4
  b17: bool = less_than b16 b10
  branch b17 %5 %6
  label %5
  b19: int = add b15 b16
  b18: int = raw_load8 b19 0
  b20: bool = equal b18 b1
  branch b20 %7 %8
  label %8
  b16: int = add b16 b3
  jump %4
  label %7
  call @27 b16
  return b6
  label %6
  b21: int = add b10 b5
  b22: int = call @16 b21
  raw_store64 b22 8 b10
  b23: int = add b22 b2
  raw_store64 b22 16 b23
  call @18 b23 b15 b10
  b6: ref = copy b22
  return b6
fn @$string_at (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b5: int binding
  b6: bool binding
  b7: bool binding
  b8: int binding
  b9: int binding
  b10: int binding
  b2: int = const 0
  b3: int = copy b0
  b4: bool = equal b3 b2
  b5: int = const 0
  branch b4 %1 %0
  label %0
  b5: int = raw_load64 b0 8
  label %1
  b6: bool = less_than b1 b2
  branch b6 %2 %3
  label %3
  b7: bool = less_than b1 b5
  branch b7 %4 %2
  label %2
  call @28 b1 b5
  return b2
  label %4
  b8: int = raw_load64 b0 16
  b9: int = add b8 b1
  b10: int = raw_load8 b9 0
  return b10
fn @$substring (p0: ref, p1: int, p2: int) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: bool binding
  b11: bool binding
  b12: bool binding
  b13: int binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: int binding
  b18: int binding
  b4: int = const 24
  b3: int = const 0
  b5: int = const 16
  b6: ref = copy b3
  b7: int = copy b0
  b8: bool = equal b7 b3
  b9: int = const 0
  branch b8 %1 %0
  label %0
  b9: int = raw_load64 b0 8
  label %1
  b10: bool = less_than b1 b3
  branch b10 %2 %3
  label %3
  b11: bool = less_than b2 b1
  branch b11 %2 %4
  label %4
  b12: bool = less_than b9 b2
  branch b12 %2 %5
  label %2
  call @29 b1 b2 b9
  return b6
  label %5
  b13: int = subtract b2 b1
  b14: int = add b13 b5
  b15: int = call @16 b14
  raw_store64 b15 8 b13
  b16: int = add b15 b4
  raw_store64 b15 16 b16
  b17: int = raw_load64 b0 16
  b18: int = add b17 b1
  call @18 b16 b18 b13
  b6: ref = copy b15
  return b6
fn @$print_string_raw (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b4: int binding
  b1: int = raw_load64 b0 8
  b2: ref = raw_load64 b0 16
  b3: int = const 1
  b4: int = raw_syscall syscall b3 b3 b2 b1
  return
fn @$print (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b4: int binding
  b5: ref binding
  b6: ref binding
  b7: int binding
  b8: int binding
  b1: int = raw_load64 b0 8
  b2: ref = raw_load64 b0 16
  b3: int = const 1
  b4: int = raw_syscall syscall b3 b3 b2 b1
  b5: ref = data $blob25
  b6: ref = raw_load64 b5 16
  b7: int = const 1
  b8: int = raw_syscall syscall b7 b7 b6 b7
  return
fn @$print_newline () -> void
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b0: ref = data $blob25
  b1: ref = raw_load64 b0 16
  b2: int = const 1
  b3: int = raw_syscall syscall b2 b2 b1 b2
  return

`

// The Phase 10.6 integer-print render, pinned: the allocator shells land
// after the program’s own functions, the digit scratch stores one byte at a
// raw offset, and each print call reaches `$print_int`.
const wantPrintIntScene = `scene
data $blob0: 8 bytes 32 32 32 32 32 32 32 32
data $blob1: 48 bytes 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255
data $blob2: 16 bytes 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255
data $blob3: 6 bytes 112 117 114 114 58 32
data $blob4: 1 bytes 10
data $blob5: 18 bytes 112 117 114 114 58 32 97 114 114 97 121 32 105 110 100 101 120 32
data $blob6: 28 bytes 32 105 115 32 111 117 116 32 111 102 32 114 97 110 103 101 32 102 111 114 32 108 101 110 103 116 104 32
data $blob7: 31 bytes 97 114 114 97 121 32 108 101 110 103 116 104 32 99 97 110 110 111 116 32 98 101 32 110 101 103 97 116 105 118 101
data $blob8: 35 bytes 97 114 114 97 121 32 101 108 101 109 101 110 116 32 115 105 122 101 32 109 117 115 116 32 98 101 32 112 111 115 105 116 105 118 101
data $blob9: 29 bytes 97 114 114 97 121 32 97 108 108 111 99 97 116 105 111 110 32 105 115 32 116 111 111 32 108 97 114 103 101
data $blob10: 38 bytes 97 114 114 97 121 32 97 99 99 101 115 115 32 111 110 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob11: 32 bytes 108 101 110 103 116 104 32 111 102 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob12: 32 bytes 97 112 112 101 110 100 32 116 111 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob13: 46 bytes 112 117 114 114 58 32 98 121 116 101 32 97 114 114 97 121 32 99 111 110 116 97 105 110 115 32 97 32 78 85 76 32 98 121 116 101 32 97 116 32 105 110 100 101 120 32
data $blob14: 18 bytes 98 121 116 101 32 97 114 114 97 121 32 105 115 32 110 117 108 108
data $blob15: 32 bytes 98 121 116 101 32 97 114 114 97 121 32 101 108 101 109 101 110 116 32 115 105 122 101 32 105 115 32 110 111 116 32 49
data $blob16: 19 bytes 112 117 114 114 58 32 115 116 114 105 110 103 32 105 110 100 101 120 32
data $blob17: 22 bytes 112 117 114 114 58 32 115 117 98 115 116 114 105 110 103 32 114 97 110 103 101 32
data $blob18: 18 bytes 112 117 114 114 58 32 115 104 105 102 116 32 99 111 117 110 116 32
data $blob19: 17 bytes 32 105 115 32 111 117 116 115 105 100 101 32 48 46 46 54 51
data $blob20: 2 bytes 46 46
data $blob21: 1 bytes 32
data $blob22: 4 bytes 116 114 117 101
data $blob23: 5 bytes 102 97 108 115 101
data $blob24: 1 bytes 48
data $blob25: 1 bytes 10
data $blob26: 1 bytes 32
data $blob27: 1 bytes 45
data $blob28: 1 bytes 48
data $blob29: 1 bytes 48
entry 0 @main
fn @main () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b0: int = const 5
  call @45 b0
  b1: int = const 0
  call @45 b1
  b2: int = const 7
  b3: int = negate b2
  call @45 b3
  b5: int = const 9223372036854775807
  b4: int = copy b5
  b6: int = const 1
  b7: int = add b4 b6
  b8: int = copy b4
  b4: int = copy b7
  call @45 b4
fn @$mmap (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b1: int = const 0
  b2: int = const 3
  b3: int = const 34
  b4: int = const 9
  b5: int = raw_syscall syscall b4 b1 b0 b2 b3
  b6: bool = less_than b5 b1
  branch b6 %0 %1
  label %0
  fail_oom
  label %1
  return b5
fn @$map () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b0: int = const 536870912
  b1: int = const 67108872
  b2: int = call @1 b0
  b3: int = call @1 b1
  b4: int = get_static $blob0 24
  b5: int = const 
  b6: int = add b2 b0
  set_static $blob1 24 b2
  set_static $blob1 32 b2
  set_static $blob1 40 b6
  set_static $blob1 48 b5
  set_static $blob1 56 b5
  set_static $blob1 64 b5
  set_static $blob2 24 b3
  set_static $blob2 32 b4
  return
fn @$pad8 (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: bool binding
  b1: int = const 15
  b2: int = const 8
  b3: int = const 16
  b4: int = add b0 b1
  b5: int = whole_divide b4 b2
  b6: int = multiply b5 b2
  b7: bool = less_than b6 b3
  branch b7 %0 %1
  label %0
  b6: int = copy b3
  label %1
  return b6
fn @$byte_set1 (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = const 1
  raw_store8 b6 0 b7
  return
fn @$byte_set0 (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = const 0
  raw_store8 b6 0 b7
  return
fn @$search (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: bool binding
  b11: int binding
  b12: bool binding
  b13: int binding
  b14: int binding
  b1: int = const 0
  b2: int = get_static $blob1 48
  b3: int = copy b1
  b4: int = copy b2
  b5: int = const 16
  label %0
  b6: bool = less_than b4 b1
  branch b6 %9 %1
  label %1
  b7: int = raw_load64 b4 0
  b8: bool = less_than b7 b0
  branch b8 %2 %3
  label %2
  b3: int = copy b4
  b4: int = raw_load64 b4 8
  jump %0
  label %3
  b9: int = raw_load64 b4 8
  b10: bool = equal b3 b1
  branch b10 %4 %5
  label %4
  set_static $blob1 48 b9
  jump %6
  label %5
  raw_store64 b3 8 b9
  label %6
  b11: int = subtract b7 b0
  b12: bool = less_than b11 b5
  branch b12 %8 %7
  label %7
  b13: int = add b4 b0
  raw_store64 b13 0 b11
  b14: int = get_static $blob1 48
  raw_store64 b13 8 b14
  set_static $blob1 48 b13
  jump %8
  label %8
  raw_store64 b4 0 b0
  call @4 b4
  return b4
  label %9
  return b1
fn @$bump (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b1: int = const 0
  b2: int = get_static $blob1 32
  b3: int = get_static $blob1 40
  b4: int = add b2 b0
  b5: bool = less_than b3 b4
  branch b5 %0 %1
  label %1
  set_static $blob1 32 b4
  raw_store64 b2 0 b0
  call @4 b2
  return b2
  label %0
  return b1
fn @$mark_at (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: bool binding
  b14: int binding
  b1: int = const 0
  b2: int = get_static $blob1 24
  b3: bool = less_than b0 b2
  branch b3 %0 %1
  label %1
  b4: int = get_static $blob1 40
  b5: bool = less_than b4 b0
  branch b5 %0 %2
  label %2
  b6: int = const 8
  b7: int = subtract b0 b2
  b8: int = whole_divide b7 b6
  b9: int = get_static $blob2 24
  b10: int = add b9 b8
  b11: int = raw_load8 b10 0
  b12: int = const 1
  b13: bool = equal b11 b12
  branch b13 %3 %0
  label %3
  b14: int = const 2
  raw_store8 b10 0 b14
  b1: int = const 1
  label %0
  return b1
fn @$ledger_at (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b1: int = const 0
  b2: int = get_static $blob1 24
  b3: bool = less_than b0 b2
  branch b3 %0 %1
  label %1
  b4: int = get_static $blob1 40
  b5: bool = less_than b4 b0
  branch b5 %0 %2
  label %2
  b6: int = const 8
  b7: int = subtract b0 b2
  b8: int = whole_divide b7 b6
  b9: int = get_static $blob2 24
  b10: int = add b9 b8
  b1: int = raw_load8 b10 0
  label %0
  return b1
fn @$scan_roots () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b0: int = get_static $blob2 32
  b1: int = stack_pointer
  b2: int = const 7
  b3: int = const 8
  b4: int = subtract b1 b2
  b4: int = whole_divide b4 b3
  b4: int = multiply b4 b3
  label %0
  b5: bool = less_than b4 b0
  branch b5 %1 %2
  label %1
  b6: int = raw_load64 b4 0
  b7: int = call @8 b6
  b4: int = add b4 b3
  jump %0
  label %2
  return
fn @$scan_heap () -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: bool binding
  b14: int binding
  b15: int binding
  b16: bool binding
  b17: int binding
  b18: int binding
  b19: bool binding
  b0: int = const 0
  b1: int = const 2
  b2: int = const 8
  b3: int = get_static $blob1 24
  b4: int = get_static $blob1 32
  b5: int = copy b3
  b6: int = copy b0
  label %0
  b19: bool = less_than b5 b4
  branch b19 %1 %2
  label %1
  b7: int = raw_load64 b5 0
  b8: int = subtract b5 b3
  b9: int = whole_divide b8 b2
  b10: int = get_static $blob2 24
  b11: int = add b10 b9
  b12: int = raw_load8 b11 0
  b13: bool = equal b12 b1
  branch b13 %3 %4
  label %3
  b14: int = add b5 b2
  b15: int = add b5 b7
  label %5
  b16: bool = less_than b14 b15
  branch b16 %6 %4
  label %6
  b17: int = raw_load64 b14 0
  b18: int = call @8 b17
  b6: int = add b6 b18
  b14: int = add b14 b2
  jump %5
  label %4
  b5: int = add b5 b7
  jump %0
  label %2
  return b6
fn @$free_block (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b2: int = const 0
  b3: int = get_static $blob1 48
  b4: int = get_static $blob1 56
  b5: int = get_static $blob1 64
  b6: bool = equal b5 b0
  branch b6 %0 %1
  label %0
  b7: int = raw_load64 b4 0
  b8: int = add b7 b1
  raw_store64 b4 0 b8
  b9: int = add b0 b1
  set_static $blob1 64 b9
  call @5 b0
  return
  label %1
  b10: int = add b0 b1
  raw_store64 b0 8 b3
  set_static $blob1 48 b0
  set_static $blob1 56 b0
  set_static $blob1 64 b10
  call @5 b0
  return
fn @$sweep_block (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: bool binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = raw_load8 b6 0
  b8: int = raw_load64 b0 0
  b9: int = const 2
  b10: int = const 1
  b11: bool = equal b7 b9
  branch b11 %0 %1
  label %0
  raw_store8 b6 0 b10
  return b8
  label %1
  b11: bool = equal b7 b10
  branch b11 %2 %3
  label %2
  call @12 b0 b8
  return b8
  label %3
  return b8
fn @$sweep () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: int binding
  b0: int = get_static $blob1 24
  b1: int = get_static $blob1 32
  b2: int = copy b0
  label %0
  b3: bool = less_than b2 b1
  branch b3 %1 %2
  label %1
  b4: int = call @13 b2
  b2: int = add b2 b4
  jump %0
  label %2
  b5: int = const 
  set_static $blob1 56 b5
  set_static $blob1 64 b5
  return
fn @$collect () -> void
  b0: int binding
  b1: int binding
  b2: bool binding
  call @10
  b0: int = const 0
  label %0
  b1: int = call @11
  b2: bool = equal b1 b0
  branch b2 %1 %0
  label %1
  call @14
  return
fn @$alloc (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: bool binding
  b1: int = const 0
  b2: int = get_static $blob1 32
  b3: bool = less_than b2 b1
  branch b3 %0 %1
  label %0
  call @2
  label %1
  b4: int = call @3 b0
  b5: int = call @6 b4
  b6: bool = equal b5 b1
  branch b6 %2 %7
  label %2
  b7: int = call @7 b4
  b8: bool = equal b7 b1
  branch b8 %3 %6
  label %3
  call @15
  b5: int = call @6 b4
  b6: bool = equal b5 b1
  branch b6 %4 %7
  label %4
  b7: int = call @7 b4
  b8: bool = equal b7 b1
  branch b8 %5 %6
  label %5
  fail_oom
  label %6
  b5: int = copy b7
  label %7
  return b5
fn @$string_length (p0: ref) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b1: int = const 0
  b2: int = copy b0
  b3: int = copy b1
  b4: bool = equal b2 b1
  branch b4 %0 %1
  label %0
  return b3
  label %1
  b3: int = raw_load64 b0 8
  return b3
fn @$copy_bytes (p0: int, p1: int, p2: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b3: int = const 0
  b4: int = const 1
  label %0
  b5: bool = less_than b3 b2
  branch b5 %2 %1
  label %2
  b6: int = add b1 b3
  b7: int = raw_load8 b6 0
  b8: int = add b0 b3
  raw_store8 b8 0 b7
  b3: int = add b3 b4
  jump %0
  label %1
  return
fn @$string_equal (p0: ref, p1: ref) -> bool
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: int binding
  b11: bool binding
  b12: bool binding
  b13: bool binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: int binding
  b18: int binding
  b19: bool binding
  b2: int = const 0
  b3: int = const 1
  b4: int = copy b0
  b5: int = copy b1
  b6: int = copy b2
  b7: int = copy b2
  b8: bool = equal b4 b2
  branch b8 %0 %1
  label %1
  b6: int = raw_load64 b0 8
  b7: int = raw_load64 b0 16
  jump %2
  label %0
  label %2
  b9: int = copy b2
  b10: int = copy b2
  b11: bool = equal b5 b2
  branch b11 %3 %4
  label %4
  b9: int = raw_load64 b1 8
  b10: int = raw_load64 b1 16
  jump %5
  label %3
  label %5
  b12: bool = equal b6 b9
  branch b12 %6 %7
  label %7
  b13: bool = const false
  return b13
  label %6
  b14: int = const 0
  label %8
  b12: bool = less_than b14 b6
  branch b12 %9 %10
  label %9
  b15: int = add b7 b14
  b16: int = raw_load8 b15 0
  b17: int = add b10 b14
  b18: int = raw_load8 b17 0
  b19: bool = equal b16 b18
  branch b19 %11 %7
  label %11
  b14: int = add b14 b3
  jump %8
  label %10
  b13: bool = const true
  return b13
fn @$string_concat (p0: ref, p1: ref) -> ref
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: bool binding
  b11: int binding
  b12: int binding
  b13: int binding
  b14: int binding
  b15: ref binding
  b16: int binding
  b17: int binding
  b18: int binding
  b2: int = const 0
  b3: int = copy b0
  b4: int = copy b2
  b5: int = copy b2
  b6: bool = equal b3 b2
  branch b6 %0 %1
  label %1
  b4: int = raw_load64 b0 8
  b5: int = raw_load64 b0 16
  jump %2
  label %0
  label %2
  b7: int = copy b1
  b8: int = copy b2
  b9: int = copy b2
  b10: bool = equal b7 b2
  branch b10 %3 %4
  label %4
  b8: int = raw_load64 b1 8
  b9: int = raw_load64 b1 16
  jump %5
  label %3
  label %5
  b11: int = add b4 b8
  b12: int = const 16
  b13: int = add b11 b12
  b14: int = call @16 b13
  b15: ref = copy b14
  b16: int = const 24
  b17: int = add b15 b16
  raw_store64 b15 8 b11
  raw_store64 b15 16 b17
  call @18 b17 b5 b4
  b18: int = add b17 b4
  call @18 b18 b9 b8
  return b15
fn @$write_byte_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  set_static $blob24 24 b1
  b2: ref = data $blob24
  b3: ref = raw_load64 b2 16
  b4: int = const 1
  b5: int = raw_syscall syscall b4 b0 b3 b4
  return
fn @$write_blob_fd (p0: int, p1: ref) -> void
  b0: int binding
  b1: ref binding
  b2: int binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b2: int = raw_load64 b1 8
  b3: ref = raw_load64 b1 16
  b4: int = const 1
  b5: int = raw_syscall syscall b4 b0 b3 b2
  return
fn @$write_digits_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b2: int = const 10
  b3: int = whole_divide b1 b2
  b4: int = whole_remainder b1 b2
  b5: int = const 0
  b6: bool = equal b3 b5
  branch b6 %0 %1
  label %1
  call @23 b0 b3
  label %0
  b7: int = negate b4
  b8: int = const 48
  b9: int = add b7 b8
  call @21 b0 b9
  return
fn @$write_int_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b2: int = const 0
  b3: bool = equal b1 b2
  branch b3 %0 %1
  label %0
  b4: int = const 48
  call @21 b0 b4
  return
  label %1
  b5: bool = less_than b1 b2
  branch b5 %2 %3
  label %2
  b6: int = const 45
  call @21 b0 b6
  call @23 b0 b1
  return
  label %3
  b7: int = negate b1
  call @23 b0 b7
  return
fn @$fail_blob (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b6: int binding
  b1: int = const 2
  b2: ref = data $blob3
  call @22 b1 b2
  call @22 b1 b0
  b3: ref = data $blob4
  call @22 b1 b3
  b4: int = const 60
  b5: int = const 1
  b6: int = raw_syscall syscall b4 b5
  return
fn @$fail_bounds (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: ref binding
  b4: ref binding
  b5: ref binding
  b6: int binding
  b7: int binding
  b8: int binding
  b2: int = const 2
  b3: ref = data $blob5
  call @22 b2 b3
  call @24 b2 b0
  b4: ref = data $blob6
  call @22 b2 b4
  call @24 b2 b1
  b5: ref = data $blob4
  call @22 b2 b5
  b6: int = const 60
  b7: int = const 1
  b8: int = raw_syscall syscall b6 b7
  return
fn @$fail_nul (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b6: int binding
  b1: int = const 2
  b2: ref = data $blob13
  call @22 b1 b2
  call @24 b1 b0
  b3: ref = data $blob4
  call @22 b1 b3
  b4: int = const 60
  b5: int = const 1
  b6: int = raw_syscall syscall b4 b5
  return
fn @$fail_string_index (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: ref binding
  b4: ref binding
  b5: ref binding
  b6: int binding
  b7: int binding
  b8: int binding
  b2: int = const 2
  b3: ref = data $blob16
  call @22 b2 b3
  call @24 b2 b0
  b4: ref = data $blob6
  call @22 b2 b4
  call @24 b2 b1
  b5: ref = data $blob4
  call @22 b2 b5
  b6: int = const 60
  b7: int = const 1
  b8: int = raw_syscall syscall b6 b7
  return
fn @$fail_substring (p0: int, p1: int, p2: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: ref binding
  b5: ref binding
  b6: ref binding
  b7: ref binding
  b8: int binding
  b9: int binding
  b10: int binding
  b3: int = const 2
  b4: ref = data $blob17
  call @22 b3 b4
  call @24 b3 b0
  b5: ref = data $blob20
  call @22 b3 b5
  call @24 b3 b1
  b6: ref = data $blob6
  call @22 b3 b6
  call @24 b3 b2
  b7: ref = data $blob4
  call @22 b3 b7
  b8: int = const 60
  b9: int = const 1
  b10: int = raw_syscall syscall b8 b9
  return
fn @$fail_shift (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: ref binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = const 2
  b2: ref = data $blob18
  call @22 b1 b2
  call @24 b1 b0
  b3: ref = data $blob19
  call @22 b1 b3
  b4: ref = data $blob4
  call @22 b1 b4
  b5: int = const 60
  b6: int = const 1
  b7: int = raw_syscall syscall b5 b6
  return
fn @$zero_bytes (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 0
  label %0
  b5: bool = less_than b2 b1
  branch b5 %1 %2
  label %1
  b6: int = add b0 b2
  raw_store8 b6 0 b4
  b2: int = add b2 b3
  jump %0
  label %2
  return
fn @$array_new (p0: int, p1: int) -> ref
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: ref binding
  b8: bool binding
  b9: ref binding
  b10: bool binding
  b11: ref binding
  b12: int binding
  b13: bool binding
  b14: ref binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: int binding
  b19: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 32
  b5: int = const 8
  b6: int = const 9223372036854775807
  b7: ref = copy b2
  b8: bool = less_than b0 b2
  branch b8 %0 %1
  label %0
  b9: ref = data $blob7
  call @25 b9
  return b7
  label %1
  b10: bool = less_than b1 b3
  branch b10 %2 %3
  label %2
  b11: ref = data $blob8
  call @25 b11
  return b7
  label %3
  b12: int = whole_divide b6 b1
  b13: bool = less_than b12 b0
  branch b13 %4 %5
  label %4
  b14: ref = data $blob9
  call @25 b14
  return b7
  label %5
  b15: int = call @16 b4
  raw_store64 b15 8 b0
  raw_store64 b15 16 b1
  raw_store64 b15 24 b2
  raw_store64 b15 32 b0
  b7: ref = copy b15
  b16: int = multiply b0 b1
  b17: bool = equal b16 b2
  branch b17 %6 %7
  label %6
  b16: int = const 1
  label %7
  b18: int = call @16 b16
  raw_store64 b15 24 b18
  b19: int = add b18 b5
  call @31 b19 b16
  return b7
fn @$array_length (p0: ref) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: ref binding
  b5: int binding
  b1: int = const 0
  b2: int = copy b0
  b3: bool = equal b2 b1
  branch b3 %0 %1
  label %0
  b4: ref = data $blob11
  call @25 b4
  return b1
  label %1
  b5: int = raw_load64 b0 8
  return b5
fn @$array_push (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: bool binding
  b8: ref binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: bool binding
  b13: int binding
  b14: bool binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: ref binding
  b19: int binding
  b20: int binding
  b21: int binding
  b22: int binding
  b23: int binding
  b24: int binding
  b25: int binding
  b26: int binding
  b27: int binding
  b28: int binding
  b29: bool binding
  b30: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 9223372036854775807
  b5: int = const 8
  b6: int = copy b0
  b7: bool = equal b6 b2
  branch b7 %0 %1
  label %0
  b8: ref = data $blob12
  call @25 b8
  return b2
  label %1
  b9: int = raw_load64 b0 8
  b10: int = raw_load64 b0 16
  b11: int = raw_load64 b0 32
  b12: bool = equal b9 b11
  branch b12 %2 %8
  label %2
  b13: int = copy b11
  b14: bool = less_than b11 b3
  branch b14 %3 %4
  label %3
  b13: int = copy b3
  jump %5
  label %4
  b15: int = add b11 b11
  b13: int = copy b15
  label %5
  b16: int = whole_divide b4 b10
  b17: bool = less_than b16 b13
  branch b17 %6 %7
  label %6
  b18: ref = data $blob9
  call @25 b18
  return b2
  label %7
  b19: int = multiply b13 b10
  b20: int = call @16 b19
  b21: int = multiply b9 b10
  b22: int = raw_load64 b0 24
  b23: int = add b20 b5
  b24: int = add b22 b5
  call @18 b23 b24 b21
  raw_store64 b0 24 b20
  raw_store64 b0 32 b13
  label %8
  b25: int = raw_load64 b0 24
  b26: int = add b25 b5
  b27: int = multiply b9 b10
  b28: int = add b26 b27
  b29: bool = equal b10 b3
  branch b29 %9 %10
  label %9
  raw_store8 b28 0 b1
  jump %11
  label %10
  raw_store64 b28 0 b1
  label %11
  b30: int = add b9 b3
  raw_store64 b0 8 b30
  return b30
fn @$array_at (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: bool binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: int binding
  b14: int binding
  b2: int = const 0
  b3: int = const 8
  b4: int = copy b0
  b5: bool = equal b4 b2
  branch b5 %0 %1
  label %0
  b6: ref = data $blob10
  call @25 b6
  return b2
  label %1
  b7: int = raw_load64 b0 8
  b8: bool = less_than b1 b2
  branch b8 %2 %3
  label %3
  b9: bool = less_than b1 b7
  branch b9 %4 %2
  label %2
  call @26 b1 b7
  return b2
  label %4
  b10: int = raw_load64 b0 16
  b11: int = raw_load64 b0 24
  b12: int = multiply b1 b10
  b13: int = add b11 b3
  b14: int = add b13 b12
  return b14
fn @$string_to_bytes (p0: ref) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: ref binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = raw_load64 b0 8
  b2: int = raw_load64 b0 16
  b3: int = const 1
  b4: ref = call @32 b1 b3
  b5: int = raw_load64 b4 24
  b6: int = const 8
  b7: int = add b5 b6
  call @18 b7 b2 b1
  return b4
fn @$bytes_to_string (p0: ref) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: ref binding
  b10: int binding
  b11: int binding
  b12: bool binding
  b13: ref binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: int binding
  b19: int binding
  b20: bool binding
  b21: int binding
  b22: int binding
  b23: int binding
  b2: int = const 24
  b1: int = const 0
  b3: int = const 1
  b4: int = const 8
  b5: int = const 16
  b6: ref = copy b1
  b7: int = copy b0
  b8: bool = equal b7 b1
  branch b8 %0 %1
  label %0
  b9: ref = data $blob14
  call @25 b9
  return b6
  label %1
  b10: int = raw_load64 b0 8
  b11: int = raw_load64 b0 16
  b12: bool = equal b11 b3
  branch b12 %2 %3
  label %3
  b13: ref = data $blob15
  call @25 b13
  return b6
  label %2
  b14: int = raw_load64 b0 24
  b15: int = add b14 b4
  b16: int = const 0
  label %4
  b17: bool = less_than b16 b10
  branch b17 %5 %6
  label %5
  b19: int = add b15 b16
  b18: int = raw_load8 b19 0
  b20: bool = equal b18 b1
  branch b20 %7 %8
  label %8
  b16: int = add b16 b3
  jump %4
  label %7
  call @27 b16
  return b6
  label %6
  b21: int = add b10 b5
  b22: int = call @16 b21
  raw_store64 b22 8 b10
  b23: int = add b22 b2
  raw_store64 b22 16 b23
  call @18 b23 b15 b10
  b6: ref = copy b22
  return b6
fn @$string_at (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b5: int binding
  b6: bool binding
  b7: bool binding
  b8: int binding
  b9: int binding
  b10: int binding
  b2: int = const 0
  b3: int = copy b0
  b4: bool = equal b3 b2
  b5: int = const 0
  branch b4 %1 %0
  label %0
  b5: int = raw_load64 b0 8
  label %1
  b6: bool = less_than b1 b2
  branch b6 %2 %3
  label %3
  b7: bool = less_than b1 b5
  branch b7 %4 %2
  label %2
  call @28 b1 b5
  return b2
  label %4
  b8: int = raw_load64 b0 16
  b9: int = add b8 b1
  b10: int = raw_load8 b9 0
  return b10
fn @$substring (p0: ref, p1: int, p2: int) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: bool binding
  b11: bool binding
  b12: bool binding
  b13: int binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: int binding
  b18: int binding
  b4: int = const 24
  b3: int = const 0
  b5: int = const 16
  b6: ref = copy b3
  b7: int = copy b0
  b8: bool = equal b7 b3
  b9: int = const 0
  branch b8 %1 %0
  label %0
  b9: int = raw_load64 b0 8
  label %1
  b10: bool = less_than b1 b3
  branch b10 %2 %3
  label %3
  b11: bool = less_than b2 b1
  branch b11 %2 %4
  label %4
  b12: bool = less_than b9 b2
  branch b12 %2 %5
  label %2
  call @29 b1 b2 b9
  return b6
  label %5
  b13: int = subtract b2 b1
  b14: int = add b13 b5
  b15: int = call @16 b14
  raw_store64 b15 8 b13
  b16: int = add b15 b4
  raw_store64 b15 16 b16
  b17: int = raw_load64 b0 16
  b18: int = add b17 b1
  call @18 b16 b18 b13
  b6: ref = copy b15
  return b6
fn @$print_string_raw (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b4: int binding
  b1: int = raw_load64 b0 8
  b2: ref = raw_load64 b0 16
  b3: int = const 1
  b4: int = raw_syscall syscall b3 b3 b2 b1
  return
fn @$print_newline () -> void
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b0: ref = data $blob25
  b1: ref = raw_load64 b0 16
  b2: int = const 1
  b3: int = raw_syscall syscall b2 b2 b1 b2
  return
fn @$write1 (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b1: int = const 1
  b2: ref = raw_load64 b0 16
  b3: int = raw_syscall syscall b1 b1 b2 b1
  return
fn @$digits (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: ref binding
  b1: int = const 10
  b2: int = whole_remainder b0 b1
  b3: int = whole_divide b0 b1
  b4: int = const 0
  b5: bool = equal b3 b4
  branch b5 %1 %0
  label %0
  call @43 b3
  label %1
  b6: int = negate b2
  b7: int = const 48
  b8: int = add b6 b7
  set_static $blob29 24 b8
  b9: ref = data $blob29
  call @42 b9
  return
fn @$print_int_raw (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: bool binding
  b3: bool binding
  b4: ref binding
  b5: ref binding
  b1: int = const 0
  b2: bool = equal b0 b1
  branch b2 %0 %1
  label %1
  b3: bool = less_than b0 b1
  branch b3 %2 %3
  label %2
  b4: ref = data $blob27
  call @42 b4
  jump %4
  label %3
  b0: int = negate b0
  label %4
  call @43 b0
  jump %5
  label %0
  b5: ref = data $blob28
  call @42 b5
  label %5
  return
fn @$print_int (p0: int) -> void
  b0: int binding
  call @44 b0
  call @41
  return

`

// The Phase 10.6 bool-print render, pinned: the bool helper branches on the
// one-byte condition and reaches the shared string "$print" routine.
const wantPrintBoolScene = `scene
data $blob0: 8 bytes 32 32 32 32 32 32 32 32
data $blob1: 48 bytes 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255
data $blob2: 16 bytes 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255 255
data $blob3: 6 bytes 112 117 114 114 58 32
data $blob4: 1 bytes 10
data $blob5: 18 bytes 112 117 114 114 58 32 97 114 114 97 121 32 105 110 100 101 120 32
data $blob6: 28 bytes 32 105 115 32 111 117 116 32 111 102 32 114 97 110 103 101 32 102 111 114 32 108 101 110 103 116 104 32
data $blob7: 31 bytes 97 114 114 97 121 32 108 101 110 103 116 104 32 99 97 110 110 111 116 32 98 101 32 110 101 103 97 116 105 118 101
data $blob8: 35 bytes 97 114 114 97 121 32 101 108 101 109 101 110 116 32 115 105 122 101 32 109 117 115 116 32 98 101 32 112 111 115 105 116 105 118 101
data $blob9: 29 bytes 97 114 114 97 121 32 97 108 108 111 99 97 116 105 111 110 32 105 115 32 116 111 111 32 108 97 114 103 101
data $blob10: 38 bytes 97 114 114 97 121 32 97 99 99 101 115 115 32 111 110 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob11: 32 bytes 108 101 110 103 116 104 32 111 102 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob12: 32 bytes 97 112 112 101 110 100 32 116 111 32 97 32 110 117 108 108 32 97 114 114 97 121 32 114 101 102 101 114 101 110 99 101
data $blob13: 46 bytes 112 117 114 114 58 32 98 121 116 101 32 97 114 114 97 121 32 99 111 110 116 97 105 110 115 32 97 32 78 85 76 32 98 121 116 101 32 97 116 32 105 110 100 101 120 32
data $blob14: 18 bytes 98 121 116 101 32 97 114 114 97 121 32 105 115 32 110 117 108 108
data $blob15: 32 bytes 98 121 116 101 32 97 114 114 97 121 32 101 108 101 109 101 110 116 32 115 105 122 101 32 105 115 32 110 111 116 32 49
data $blob16: 19 bytes 112 117 114 114 58 32 115 116 114 105 110 103 32 105 110 100 101 120 32
data $blob17: 22 bytes 112 117 114 114 58 32 115 117 98 115 116 114 105 110 103 32 114 97 110 103 101 32
data $blob18: 18 bytes 112 117 114 114 58 32 115 104 105 102 116 32 99 111 117 110 116 32
data $blob19: 17 bytes 32 105 115 32 111 117 116 115 105 100 101 32 48 46 46 54 51
data $blob20: 2 bytes 46 46
data $blob21: 1 bytes 32
data $blob22: 4 bytes 116 114 117 101
data $blob23: 5 bytes 102 97 108 115 101
data $blob24: 1 bytes 48
data $blob25: 1 bytes 10
data $blob26: 1 bytes 32
data $blob27: 4 bytes 116 114 117 101
data $blob28: 5 bytes 102 97 108 115 101
data $blob29: 4 bytes 116 114 117 101
data $blob30: 5 bytes 102 97 108 115 101
entry 0 @main
fn @main () -> void
  b0: bool binding
  b1: bool binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b5: bool binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: bool binding
  b10: bool binding
  b11: int binding
  b12: int binding
  b13: bool binding
  b0: bool = const true
  call @43 b0
  b1: bool = const false
  call @43 b1
  b2: int = const 1
  b3: int = const 2
  b4: bool = less_than b2 b3
  call @43 b4
  b5: bool = const true
  branch b5 %0 %1
  label %1
  b6: bool = const true
  jump %2
  label %0
  b6: bool = const false
  label %2
  call @43 b6
  b7: int = const 3
  b8: int = const 3
  b9: bool = equal b7 b8
  branch b9 %3 %4
  label %4
  b10: bool = const false
  jump %5
  label %3
  b11: int = const 4
  b12: int = const 10
  b13: bool = less_than b11 b12
  b10: bool = copy b13
  label %5
  call @43 b10
fn @$mmap (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b1: int = const 0
  b2: int = const 3
  b3: int = const 34
  b4: int = const 9
  b5: int = raw_syscall syscall b4 b1 b0 b2 b3
  b6: bool = less_than b5 b1
  branch b6 %0 %1
  label %0
  fail_oom
  label %1
  return b5
fn @$map () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b0: int = const 536870912
  b1: int = const 67108872
  b2: int = call @1 b0
  b3: int = call @1 b1
  b4: int = get_static $blob0 24
  b5: int = const 
  b6: int = add b2 b0
  set_static $blob1 24 b2
  set_static $blob1 32 b2
  set_static $blob1 40 b6
  set_static $blob1 48 b5
  set_static $blob1 56 b5
  set_static $blob1 64 b5
  set_static $blob2 24 b3
  set_static $blob2 32 b4
  return
fn @$pad8 (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: bool binding
  b1: int = const 15
  b2: int = const 8
  b3: int = const 16
  b4: int = add b0 b1
  b5: int = whole_divide b4 b2
  b6: int = multiply b5 b2
  b7: bool = less_than b6 b3
  branch b7 %0 %1
  label %0
  b6: int = copy b3
  label %1
  return b6
fn @$byte_set1 (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = const 1
  raw_store8 b6 0 b7
  return
fn @$byte_set0 (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = const 0
  raw_store8 b6 0 b7
  return
fn @$search (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: bool binding
  b11: int binding
  b12: bool binding
  b13: int binding
  b14: int binding
  b1: int = const 0
  b2: int = get_static $blob1 48
  b3: int = copy b1
  b4: int = copy b2
  b5: int = const 16
  label %0
  b6: bool = less_than b4 b1
  branch b6 %9 %1
  label %1
  b7: int = raw_load64 b4 0
  b8: bool = less_than b7 b0
  branch b8 %2 %3
  label %2
  b3: int = copy b4
  b4: int = raw_load64 b4 8
  jump %0
  label %3
  b9: int = raw_load64 b4 8
  b10: bool = equal b3 b1
  branch b10 %4 %5
  label %4
  set_static $blob1 48 b9
  jump %6
  label %5
  raw_store64 b3 8 b9
  label %6
  b11: int = subtract b7 b0
  b12: bool = less_than b11 b5
  branch b12 %8 %7
  label %7
  b13: int = add b4 b0
  raw_store64 b13 0 b11
  b14: int = get_static $blob1 48
  raw_store64 b13 8 b14
  set_static $blob1 48 b13
  jump %8
  label %8
  raw_store64 b4 0 b0
  call @4 b4
  return b4
  label %9
  return b1
fn @$bump (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b1: int = const 0
  b2: int = get_static $blob1 32
  b3: int = get_static $blob1 40
  b4: int = add b2 b0
  b5: bool = less_than b3 b4
  branch b5 %0 %1
  label %1
  set_static $blob1 32 b4
  raw_store64 b2 0 b0
  call @4 b2
  return b2
  label %0
  return b1
fn @$mark_at (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: bool binding
  b14: int binding
  b1: int = const 0
  b2: int = get_static $blob1 24
  b3: bool = less_than b0 b2
  branch b3 %0 %1
  label %1
  b4: int = get_static $blob1 40
  b5: bool = less_than b4 b0
  branch b5 %0 %2
  label %2
  b6: int = const 8
  b7: int = subtract b0 b2
  b8: int = whole_divide b7 b6
  b9: int = get_static $blob2 24
  b10: int = add b9 b8
  b11: int = raw_load8 b10 0
  b12: int = const 1
  b13: bool = equal b11 b12
  branch b13 %3 %0
  label %3
  b14: int = const 2
  raw_store8 b10 0 b14
  b1: int = const 1
  label %0
  return b1
fn @$ledger_at (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b1: int = const 0
  b2: int = get_static $blob1 24
  b3: bool = less_than b0 b2
  branch b3 %0 %1
  label %1
  b4: int = get_static $blob1 40
  b5: bool = less_than b4 b0
  branch b5 %0 %2
  label %2
  b6: int = const 8
  b7: int = subtract b0 b2
  b8: int = whole_divide b7 b6
  b9: int = get_static $blob2 24
  b10: int = add b9 b8
  b1: int = raw_load8 b10 0
  label %0
  return b1
fn @$scan_roots () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b0: int = get_static $blob2 32
  b1: int = stack_pointer
  b2: int = const 7
  b3: int = const 8
  b4: int = subtract b1 b2
  b4: int = whole_divide b4 b3
  b4: int = multiply b4 b3
  label %0
  b5: bool = less_than b4 b0
  branch b5 %1 %2
  label %1
  b6: int = raw_load64 b4 0
  b7: int = call @8 b6
  b4: int = add b4 b3
  jump %0
  label %2
  return
fn @$scan_heap () -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: bool binding
  b14: int binding
  b15: int binding
  b16: bool binding
  b17: int binding
  b18: int binding
  b19: bool binding
  b0: int = const 0
  b1: int = const 2
  b2: int = const 8
  b3: int = get_static $blob1 24
  b4: int = get_static $blob1 32
  b5: int = copy b3
  b6: int = copy b0
  label %0
  b19: bool = less_than b5 b4
  branch b19 %1 %2
  label %1
  b7: int = raw_load64 b5 0
  b8: int = subtract b5 b3
  b9: int = whole_divide b8 b2
  b10: int = get_static $blob2 24
  b11: int = add b10 b9
  b12: int = raw_load8 b11 0
  b13: bool = equal b12 b1
  branch b13 %3 %4
  label %3
  b14: int = add b5 b2
  b15: int = add b5 b7
  label %5
  b16: bool = less_than b14 b15
  branch b16 %6 %4
  label %6
  b17: int = raw_load64 b14 0
  b18: int = call @8 b17
  b6: int = add b6 b18
  b14: int = add b14 b2
  jump %5
  label %4
  b5: int = add b5 b7
  jump %0
  label %2
  return b6
fn @$free_block (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b2: int = const 0
  b3: int = get_static $blob1 48
  b4: int = get_static $blob1 56
  b5: int = get_static $blob1 64
  b6: bool = equal b5 b0
  branch b6 %0 %1
  label %0
  b7: int = raw_load64 b4 0
  b8: int = add b7 b1
  raw_store64 b4 0 b8
  b9: int = add b0 b1
  set_static $blob1 64 b9
  call @5 b0
  return
  label %1
  b10: int = add b0 b1
  raw_store64 b0 8 b3
  set_static $blob1 48 b0
  set_static $blob1 56 b0
  set_static $blob1 64 b10
  call @5 b0
  return
fn @$sweep_block (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: int binding
  b11: bool binding
  b1: int = get_static $blob1 24
  b2: int = subtract b0 b1
  b3: int = const 8
  b4: int = whole_divide b2 b3
  b5: int = get_static $blob2 24
  b6: int = add b5 b4
  b7: int = raw_load8 b6 0
  b8: int = raw_load64 b0 0
  b9: int = const 2
  b10: int = const 1
  b11: bool = equal b7 b9
  branch b11 %0 %1
  label %0
  raw_store8 b6 0 b10
  return b8
  label %1
  b11: bool = equal b7 b10
  branch b11 %2 %3
  label %2
  call @12 b0 b8
  return b8
  label %3
  return b8
fn @$sweep () -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: int binding
  b0: int = get_static $blob1 24
  b1: int = get_static $blob1 32
  b2: int = copy b0
  label %0
  b3: bool = less_than b2 b1
  branch b3 %1 %2
  label %1
  b4: int = call @13 b2
  b2: int = add b2 b4
  jump %0
  label %2
  b5: int = const 
  set_static $blob1 56 b5
  set_static $blob1 64 b5
  return
fn @$collect () -> void
  b0: int binding
  b1: int binding
  b2: bool binding
  call @10
  b0: int = const 0
  label %0
  b1: int = call @11
  b2: bool = equal b1 b0
  branch b2 %1 %0
  label %1
  call @14
  return
fn @$alloc (p0: int) -> int
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: bool binding
  b1: int = const 0
  b2: int = get_static $blob1 32
  b3: bool = less_than b2 b1
  branch b3 %0 %1
  label %0
  call @2
  label %1
  b4: int = call @3 b0
  b5: int = call @6 b4
  b6: bool = equal b5 b1
  branch b6 %2 %7
  label %2
  b7: int = call @7 b4
  b8: bool = equal b7 b1
  branch b8 %3 %6
  label %3
  call @15
  b5: int = call @6 b4
  b6: bool = equal b5 b1
  branch b6 %4 %7
  label %4
  b7: int = call @7 b4
  b8: bool = equal b7 b1
  branch b8 %5 %6
  label %5
  fail_oom
  label %6
  b5: int = copy b7
  label %7
  return b5
fn @$string_length (p0: ref) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b1: int = const 0
  b2: int = copy b0
  b3: int = copy b1
  b4: bool = equal b2 b1
  branch b4 %0 %1
  label %0
  return b3
  label %1
  b3: int = raw_load64 b0 8
  return b3
fn @$copy_bytes (p0: int, p1: int, p2: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b8: int binding
  b3: int = const 0
  b4: int = const 1
  label %0
  b5: bool = less_than b3 b2
  branch b5 %2 %1
  label %2
  b6: int = add b1 b3
  b7: int = raw_load8 b6 0
  b8: int = add b0 b3
  raw_store8 b8 0 b7
  b3: int = add b3 b4
  jump %0
  label %1
  return
fn @$string_equal (p0: ref, p1: ref) -> bool
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: int binding
  b11: bool binding
  b12: bool binding
  b13: bool binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: int binding
  b18: int binding
  b19: bool binding
  b2: int = const 0
  b3: int = const 1
  b4: int = copy b0
  b5: int = copy b1
  b6: int = copy b2
  b7: int = copy b2
  b8: bool = equal b4 b2
  branch b8 %0 %1
  label %1
  b6: int = raw_load64 b0 8
  b7: int = raw_load64 b0 16
  jump %2
  label %0
  label %2
  b9: int = copy b2
  b10: int = copy b2
  b11: bool = equal b5 b2
  branch b11 %3 %4
  label %4
  b9: int = raw_load64 b1 8
  b10: int = raw_load64 b1 16
  jump %5
  label %3
  label %5
  b12: bool = equal b6 b9
  branch b12 %6 %7
  label %7
  b13: bool = const false
  return b13
  label %6
  b14: int = const 0
  label %8
  b12: bool = less_than b14 b6
  branch b12 %9 %10
  label %9
  b15: int = add b7 b14
  b16: int = raw_load8 b15 0
  b17: int = add b10 b14
  b18: int = raw_load8 b17 0
  b19: bool = equal b16 b18
  branch b19 %11 %7
  label %11
  b14: int = add b14 b3
  jump %8
  label %10
  b13: bool = const true
  return b13
fn @$string_concat (p0: ref, p1: ref) -> ref
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b10: bool binding
  b11: int binding
  b12: int binding
  b13: int binding
  b14: int binding
  b15: ref binding
  b16: int binding
  b17: int binding
  b18: int binding
  b2: int = const 0
  b3: int = copy b0
  b4: int = copy b2
  b5: int = copy b2
  b6: bool = equal b3 b2
  branch b6 %0 %1
  label %1
  b4: int = raw_load64 b0 8
  b5: int = raw_load64 b0 16
  jump %2
  label %0
  label %2
  b7: int = copy b1
  b8: int = copy b2
  b9: int = copy b2
  b10: bool = equal b7 b2
  branch b10 %3 %4
  label %4
  b8: int = raw_load64 b1 8
  b9: int = raw_load64 b1 16
  jump %5
  label %3
  label %5
  b11: int = add b4 b8
  b12: int = const 16
  b13: int = add b11 b12
  b14: int = call @16 b13
  b15: ref = copy b14
  b16: int = const 24
  b17: int = add b15 b16
  raw_store64 b15 8 b11
  raw_store64 b15 16 b17
  call @18 b17 b5 b4
  b18: int = add b17 b4
  call @18 b18 b9 b8
  return b15
fn @$write_byte_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  set_static $blob24 24 b1
  b2: ref = data $blob24
  b3: ref = raw_load64 b2 16
  b4: int = const 1
  b5: int = raw_syscall syscall b4 b0 b3 b4
  return
fn @$write_blob_fd (p0: int, p1: ref) -> void
  b0: int binding
  b1: ref binding
  b2: int binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b2: int = raw_load64 b1 8
  b3: ref = raw_load64 b1 16
  b4: int = const 1
  b5: int = raw_syscall syscall b4 b0 b3 b2
  return
fn @$write_digits_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: bool binding
  b7: int binding
  b8: int binding
  b9: int binding
  b2: int = const 10
  b3: int = whole_divide b1 b2
  b4: int = whole_remainder b1 b2
  b5: int = const 0
  b6: bool = equal b3 b5
  branch b6 %0 %1
  label %1
  call @23 b0 b3
  label %0
  b7: int = negate b4
  b8: int = const 48
  b9: int = add b7 b8
  call @21 b0 b9
  return
fn @$write_int_fd (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b7: int binding
  b2: int = const 0
  b3: bool = equal b1 b2
  branch b3 %0 %1
  label %0
  b4: int = const 48
  call @21 b0 b4
  return
  label %1
  b5: bool = less_than b1 b2
  branch b5 %2 %3
  label %2
  b6: int = const 45
  call @21 b0 b6
  call @23 b0 b1
  return
  label %3
  b7: int = negate b1
  call @23 b0 b7
  return
fn @$fail_blob (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b6: int binding
  b1: int = const 2
  b2: ref = data $blob3
  call @22 b1 b2
  call @22 b1 b0
  b3: ref = data $blob4
  call @22 b1 b3
  b4: int = const 60
  b5: int = const 1
  b6: int = raw_syscall syscall b4 b5
  return
fn @$fail_bounds (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: ref binding
  b4: ref binding
  b5: ref binding
  b6: int binding
  b7: int binding
  b8: int binding
  b2: int = const 2
  b3: ref = data $blob5
  call @22 b2 b3
  call @24 b2 b0
  b4: ref = data $blob6
  call @22 b2 b4
  call @24 b2 b1
  b5: ref = data $blob4
  call @22 b2 b5
  b6: int = const 60
  b7: int = const 1
  b8: int = raw_syscall syscall b6 b7
  return
fn @$fail_nul (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: int binding
  b5: int binding
  b6: int binding
  b1: int = const 2
  b2: ref = data $blob13
  call @22 b1 b2
  call @24 b1 b0
  b3: ref = data $blob4
  call @22 b1 b3
  b4: int = const 60
  b5: int = const 1
  b6: int = raw_syscall syscall b4 b5
  return
fn @$fail_string_index (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: ref binding
  b4: ref binding
  b5: ref binding
  b6: int binding
  b7: int binding
  b8: int binding
  b2: int = const 2
  b3: ref = data $blob16
  call @22 b2 b3
  call @24 b2 b0
  b4: ref = data $blob6
  call @22 b2 b4
  call @24 b2 b1
  b5: ref = data $blob4
  call @22 b2 b5
  b6: int = const 60
  b7: int = const 1
  b8: int = raw_syscall syscall b6 b7
  return
fn @$fail_substring (p0: int, p1: int, p2: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: ref binding
  b5: ref binding
  b6: ref binding
  b7: ref binding
  b8: int binding
  b9: int binding
  b10: int binding
  b3: int = const 2
  b4: ref = data $blob17
  call @22 b3 b4
  call @24 b3 b0
  b5: ref = data $blob20
  call @22 b3 b5
  call @24 b3 b1
  b6: ref = data $blob6
  call @22 b3 b6
  call @24 b3 b2
  b7: ref = data $blob4
  call @22 b3 b7
  b8: int = const 60
  b9: int = const 1
  b10: int = raw_syscall syscall b8 b9
  return
fn @$fail_shift (p0: int) -> void
  b0: int binding
  b1: int binding
  b2: ref binding
  b3: ref binding
  b4: ref binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = const 2
  b2: ref = data $blob18
  call @22 b1 b2
  call @24 b1 b0
  b3: ref = data $blob19
  call @22 b1 b3
  b4: ref = data $blob4
  call @22 b1 b4
  b5: int = const 60
  b6: int = const 1
  b7: int = raw_syscall syscall b5 b6
  return
fn @$zero_bytes (p0: int, p1: int) -> void
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 0
  label %0
  b5: bool = less_than b2 b1
  branch b5 %1 %2
  label %1
  b6: int = add b0 b2
  raw_store8 b6 0 b4
  b2: int = add b2 b3
  jump %0
  label %2
  return
fn @$array_new (p0: int, p1: int) -> ref
  b0: int binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: ref binding
  b8: bool binding
  b9: ref binding
  b10: bool binding
  b11: ref binding
  b12: int binding
  b13: bool binding
  b14: ref binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: int binding
  b19: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 32
  b5: int = const 8
  b6: int = const 9223372036854775807
  b7: ref = copy b2
  b8: bool = less_than b0 b2
  branch b8 %0 %1
  label %0
  b9: ref = data $blob7
  call @25 b9
  return b7
  label %1
  b10: bool = less_than b1 b3
  branch b10 %2 %3
  label %2
  b11: ref = data $blob8
  call @25 b11
  return b7
  label %3
  b12: int = whole_divide b6 b1
  b13: bool = less_than b12 b0
  branch b13 %4 %5
  label %4
  b14: ref = data $blob9
  call @25 b14
  return b7
  label %5
  b15: int = call @16 b4
  raw_store64 b15 8 b0
  raw_store64 b15 16 b1
  raw_store64 b15 24 b2
  raw_store64 b15 32 b0
  b7: ref = copy b15
  b16: int = multiply b0 b1
  b17: bool = equal b16 b2
  branch b17 %6 %7
  label %6
  b16: int = const 1
  label %7
  b18: int = call @16 b16
  raw_store64 b15 24 b18
  b19: int = add b18 b5
  call @31 b19 b16
  return b7
fn @$array_length (p0: ref) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: bool binding
  b4: ref binding
  b5: int binding
  b1: int = const 0
  b2: int = copy b0
  b3: bool = equal b2 b1
  branch b3 %0 %1
  label %0
  b4: ref = data $blob11
  call @25 b4
  return b1
  label %1
  b5: int = raw_load64 b0 8
  return b5
fn @$array_push (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: int binding
  b7: bool binding
  b8: ref binding
  b9: int binding
  b10: int binding
  b11: int binding
  b12: bool binding
  b13: int binding
  b14: bool binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: ref binding
  b19: int binding
  b20: int binding
  b21: int binding
  b22: int binding
  b23: int binding
  b24: int binding
  b25: int binding
  b26: int binding
  b27: int binding
  b28: int binding
  b29: bool binding
  b30: int binding
  b2: int = const 0
  b3: int = const 1
  b4: int = const 9223372036854775807
  b5: int = const 8
  b6: int = copy b0
  b7: bool = equal b6 b2
  branch b7 %0 %1
  label %0
  b8: ref = data $blob12
  call @25 b8
  return b2
  label %1
  b9: int = raw_load64 b0 8
  b10: int = raw_load64 b0 16
  b11: int = raw_load64 b0 32
  b12: bool = equal b9 b11
  branch b12 %2 %8
  label %2
  b13: int = copy b11
  b14: bool = less_than b11 b3
  branch b14 %3 %4
  label %3
  b13: int = copy b3
  jump %5
  label %4
  b15: int = add b11 b11
  b13: int = copy b15
  label %5
  b16: int = whole_divide b4 b10
  b17: bool = less_than b16 b13
  branch b17 %6 %7
  label %6
  b18: ref = data $blob9
  call @25 b18
  return b2
  label %7
  b19: int = multiply b13 b10
  b20: int = call @16 b19
  b21: int = multiply b9 b10
  b22: int = raw_load64 b0 24
  b23: int = add b20 b5
  b24: int = add b22 b5
  call @18 b23 b24 b21
  raw_store64 b0 24 b20
  raw_store64 b0 32 b13
  label %8
  b25: int = raw_load64 b0 24
  b26: int = add b25 b5
  b27: int = multiply b9 b10
  b28: int = add b26 b27
  b29: bool = equal b10 b3
  branch b29 %9 %10
  label %9
  raw_store8 b28 0 b1
  jump %11
  label %10
  raw_store64 b28 0 b1
  label %11
  b30: int = add b9 b3
  raw_store64 b0 8 b30
  return b30
fn @$array_at (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: bool binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: bool binding
  b10: int binding
  b11: int binding
  b12: int binding
  b13: int binding
  b14: int binding
  b2: int = const 0
  b3: int = const 8
  b4: int = copy b0
  b5: bool = equal b4 b2
  branch b5 %0 %1
  label %0
  b6: ref = data $blob10
  call @25 b6
  return b2
  label %1
  b7: int = raw_load64 b0 8
  b8: bool = less_than b1 b2
  branch b8 %2 %3
  label %3
  b9: bool = less_than b1 b7
  branch b9 %4 %2
  label %2
  call @26 b1 b7
  return b2
  label %4
  b10: int = raw_load64 b0 16
  b11: int = raw_load64 b0 24
  b12: int = multiply b1 b10
  b13: int = add b11 b3
  b14: int = add b13 b12
  return b14
fn @$string_to_bytes (p0: ref) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: ref binding
  b5: int binding
  b6: int binding
  b7: int binding
  b1: int = raw_load64 b0 8
  b2: int = raw_load64 b0 16
  b3: int = const 1
  b4: ref = call @32 b1 b3
  b5: int = raw_load64 b4 24
  b6: int = const 8
  b7: int = add b5 b6
  call @18 b7 b2 b1
  return b4
fn @$bytes_to_string (p0: ref) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: ref binding
  b10: int binding
  b11: int binding
  b12: bool binding
  b13: ref binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: bool binding
  b18: int binding
  b19: int binding
  b20: bool binding
  b21: int binding
  b22: int binding
  b23: int binding
  b2: int = const 24
  b1: int = const 0
  b3: int = const 1
  b4: int = const 8
  b5: int = const 16
  b6: ref = copy b1
  b7: int = copy b0
  b8: bool = equal b7 b1
  branch b8 %0 %1
  label %0
  b9: ref = data $blob14
  call @25 b9
  return b6
  label %1
  b10: int = raw_load64 b0 8
  b11: int = raw_load64 b0 16
  b12: bool = equal b11 b3
  branch b12 %2 %3
  label %3
  b13: ref = data $blob15
  call @25 b13
  return b6
  label %2
  b14: int = raw_load64 b0 24
  b15: int = add b14 b4
  b16: int = const 0
  label %4
  b17: bool = less_than b16 b10
  branch b17 %5 %6
  label %5
  b19: int = add b15 b16
  b18: int = raw_load8 b19 0
  b20: bool = equal b18 b1
  branch b20 %7 %8
  label %8
  b16: int = add b16 b3
  jump %4
  label %7
  call @27 b16
  return b6
  label %6
  b21: int = add b10 b5
  b22: int = call @16 b21
  raw_store64 b22 8 b10
  b23: int = add b22 b2
  raw_store64 b22 16 b23
  call @18 b23 b15 b10
  b6: ref = copy b22
  return b6
fn @$string_at (p0: ref, p1: int) -> int
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: bool binding
  b5: int binding
  b6: bool binding
  b7: bool binding
  b8: int binding
  b9: int binding
  b10: int binding
  b2: int = const 0
  b3: int = copy b0
  b4: bool = equal b3 b2
  b5: int = const 0
  branch b4 %1 %0
  label %0
  b5: int = raw_load64 b0 8
  label %1
  b6: bool = less_than b1 b2
  branch b6 %2 %3
  label %3
  b7: bool = less_than b1 b5
  branch b7 %4 %2
  label %2
  call @28 b1 b5
  return b2
  label %4
  b8: int = raw_load64 b0 16
  b9: int = add b8 b1
  b10: int = raw_load8 b9 0
  return b10
fn @$substring (p0: ref, p1: int, p2: int) -> ref
  b0: ref binding
  b1: int binding
  b2: int binding
  b3: int binding
  b4: int binding
  b5: int binding
  b6: ref binding
  b7: int binding
  b8: bool binding
  b9: int binding
  b10: bool binding
  b11: bool binding
  b12: bool binding
  b13: int binding
  b14: int binding
  b15: int binding
  b16: int binding
  b17: int binding
  b18: int binding
  b4: int = const 24
  b3: int = const 0
  b5: int = const 16
  b6: ref = copy b3
  b7: int = copy b0
  b8: bool = equal b7 b3
  b9: int = const 0
  branch b8 %1 %0
  label %0
  b9: int = raw_load64 b0 8
  label %1
  b10: bool = less_than b1 b3
  branch b10 %2 %3
  label %3
  b11: bool = less_than b2 b1
  branch b11 %2 %4
  label %4
  b12: bool = less_than b9 b2
  branch b12 %2 %5
  label %2
  call @29 b1 b2 b9
  return b6
  label %5
  b13: int = subtract b2 b1
  b14: int = add b13 b5
  b15: int = call @16 b14
  raw_store64 b15 8 b13
  b16: int = add b15 b4
  raw_store64 b15 16 b16
  b17: int = raw_load64 b0 16
  b18: int = add b17 b1
  call @18 b16 b18 b13
  b6: ref = copy b15
  return b6
fn @$print_string_raw (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b4: int binding
  b1: int = raw_load64 b0 8
  b2: ref = raw_load64 b0 16
  b3: int = const 1
  b4: int = raw_syscall syscall b3 b3 b2 b1
  return
fn @$print (p0: ref) -> void
  b0: ref binding
  b1: int binding
  b2: ref binding
  b3: int binding
  b4: int binding
  b5: ref binding
  b6: ref binding
  b7: int binding
  b8: int binding
  b1: int = raw_load64 b0 8
  b2: ref = raw_load64 b0 16
  b3: int = const 1
  b4: int = raw_syscall syscall b3 b3 b2 b1
  b5: ref = data $blob25
  b6: ref = raw_load64 b5 16
  b7: int = const 1
  b8: int = raw_syscall syscall b7 b7 b6 b7
  return
fn @$print_bool_raw (p0: bool) -> void
  b0: bool binding
  b1: ref binding
  branch b0 %0 %1
  label %0
  b1: ref = data $blob27
  call @40 b1
  return
  label %1
  b1: ref = data $blob28
  call @40 b1
  return
fn @$print_bool (p0: bool) -> void
  b0: bool binding
  b1: ref binding
  branch b0 %0 %1
  label %0
  b1: ref = data $blob29
  call @41 b1
  return
  label %1
  b1: ref = data $blob30
  call @41 b1
  return
fn @$print_newline () -> void
  b0: ref binding
  b1: ref binding
  b2: int binding
  b3: int binding
  b0: ref = data $blob25
  b1: ref = raw_load64 b0 16
  b2: int = const 1
  b3: int = raw_syscall syscall b2 b2 b1 b2
  return

`

// lowerCorpusSources returns the scalar corpus programs the lowering runs.
func lowerCorpusSources(t *testing.T) []string {
	t.Helper()
	sources, err := filepath.Glob(repoPath(t, "..", "..", "tests", "native", "*.pcy"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("globbing the native corpus: %v (%d sources)", err, len(sources))
	}
	return sources
}

// TestNativeProbeLowerCorpus lowers every corpus program into a static ELF
// with no C compiler, runs it with an empty PATH, and demands the output,
// standard error, and status of the C pipeline's build of the same source;
// the Percy pipeline's production is byte-compared in its own test. The
// sources stay deliberately small because the selector's slice frame carries
// at most fourteen locals per function.
func TestNativeProbeLowerCorpus(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	purr := goPurrReference(t)
	for _, source := range lowerCorpusSources(t) {
		name := strings.TrimSuffix(filepath.Base(source), ".pcy")
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			cRun := runExternal(t, purr, []string{"run", source}, directory, nil)

			native := filepath.Join(directory, name)
			written := runExternal(t, executable, []string{"lower", source, native}, directory, nil)
			if written.status != 0 || written.stderr != "" || written.stdout != "built "+native+"\n" {
				t.Fatalf("lower: status %d, stdout %q, stderr %q", written.status, written.stdout, written.stderr)
			}
			if err := os.Chmod(native, 0o755); err != nil {
				t.Fatalf("chmod native: %v", err)
			}

			run := runExternal(t, native, nil, directory, []string{"PATH=/nonexistent"})
			if run.status != cRun.status || run.stdout != cRun.stdout || run.stderr != cRun.stderr {
				t.Fatalf("native behavior differs:\nC: %d %q %q\nnative: %d %q %q",
					cRun.status, cRun.stdout, cRun.stderr, run.status, run.stdout, run.stderr)
			}

			// The production is deterministic: a rebuild is byte-identical.
			again := filepath.Join(directory, name+"-again")
			if written := runExternal(t, executable, []string{"lower", source, again}, directory, nil); written.status != 0 {
				t.Fatalf("rebuild status %d", written.status)
			}
			first, err := os.ReadFile(native)
			if err != nil {
				t.Fatalf("reading the first ELF: %v", err)
			}
			second, err := os.ReadFile(again)
			if err != nil {
				t.Fatalf("reading the rebuilt ELF: %v", err)
			}
			if string(first) != string(second) {
				t.Fatalf("two productions of %s differ in %d bytes", name, len(first))
			}
		})
	}
}

// TestNativeProbeLowerModules runs the real multi-file tests/modules tree
// through the native backend: imported structs and public/private functions,
// cross-module calls, and multi-argument print, compared with the C
// pipeline's run and checked for deterministic production.
func TestNativeProbeLowerModules(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	purr := goPurrReference(t)
	source := repoPath(t, "..", "..", "tests", "modules", "main.pcy")
	directory := t.TempDir()

	cRun := runExternal(t, purr, []string{"run", source}, directory, nil)

	native := filepath.Join(directory, "modules")
	written := runExternal(t, executable, []string{"lower", source, native}, directory, nil)
	if written.status != 0 || written.stderr != "" || written.stdout != "built "+native+"\n" {
		t.Fatalf("lower modules: status %d, stdout %q, stderr %q", written.status, written.stdout, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	run := runExternal(t, native, nil, directory, []string{"PATH=/nonexistent"})
	if run.status != cRun.status || run.stdout != cRun.stdout || run.stderr != cRun.stderr {
		t.Fatalf("native modules differ:\nC: %d %q %q\nnative: %d %q %q",
			cRun.status, cRun.stdout, cRun.stderr, run.status, run.stdout, run.stderr)
	}

	again := filepath.Join(directory, "modules-again")
	if written := runExternal(t, executable, []string{"lower", source, again}, directory, nil); written.status != 0 {
		t.Fatalf("second modules write status %d", written.status)
	}
	first, err := os.ReadFile(native)
	if err != nil {
		t.Fatalf("reading the first ELF: %v", err)
	}
	secondFile, err := os.ReadFile(again)
	if err != nil {
		t.Fatalf("reading the second ELF: %v", err)
	}
	if string(first) != string(secondFile) {
		t.Fatalf("two module productions differ in %d bytes", len(first))
	}
}

// TestNativeProbeReadFile runs a path-taking try_read_file program through
// both pipelines over an existing file, an empty file, a directory, and a
// missing path. The program prints only the result, so the comparison covers
// Ok text bytes and the exact open/read failure texts.
func TestNativeProbeReadFile(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	purr := goPurrReference(t)
	directory := t.TempDir()

	source := filepath.Join(directory, "readarg.pcy")
	program := `fn main() {
    string[] args = argv();
    match (try_read_file(args[1])) {
        Ok(text) {
            print("ok", length(text), text);
        }
        Failed(message) {
            print("fail", message);
        }
    }
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatalf("writing the read source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "data.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("writing data.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "empty.txt"), nil, 0o644); err != nil {
		t.Fatalf("writing empty.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(directory, "dir"), 0o755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	cBinary := filepath.Join(directory, "read_c")
	built := runExternal(t, purr, []string{"build", source, "-o", cBinary}, directory, nil)
	if built.status != 0 {
		t.Fatalf("C build: status %d, stderr %q", built.status, built.stderr)
	}
	native := filepath.Join(directory, "read_native")
	written := runExternal(t, executable, []string{"lower", source, native}, directory, nil)
	if written.status != 0 || written.stderr != "" {
		t.Fatalf("native lower: status %d, stderr %q", written.status, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	for _, path := range []string{"data.txt", "empty.txt", "dir", "missing.txt"} {
		t.Run(path, func(t *testing.T) {
			cRun := runExternal(t, cBinary, []string{path}, directory, nil)
			nativeRun := runExternal(t, native, []string{path}, directory, []string{"PATH=/nonexistent"})
			if nativeRun.status != cRun.status || nativeRun.stdout != cRun.stdout || nativeRun.stderr != cRun.stderr {
				t.Fatalf("read %s differs:\nC: %d %q %q\nnative: %d %q %q",
					path, cRun.status, cRun.stdout, cRun.stderr, nativeRun.status, nativeRun.stdout, nativeRun.stderr)
			}
		})
	}
}

// TestNativeProbeReadFileFatal runs the fatal read_file builtin through both
// pipelines over a regular file, an empty file, a directory, a missing path,
// and a sparse file above the 256 MiB limit. stdout, stderr, and status must
// agree, including the libc-cased strerror text in the fatal messages.
func TestNativeProbeReadFileFatal(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	purr := goPurrReference(t)
	directory := t.TempDir()

	source := filepath.Join(directory, "readfatal.pcy")
	program := `fn main() {
    string[] arguments = argv();
    string text = read_file(arguments[1]);
    print(length(text));
    if (length(text) > 0) {
        print(text[0]);
    }
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatalf("writing the read source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "data.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("writing data.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "empty.txt"), nil, 0o644); err != nil {
		t.Fatalf("writing empty.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(directory, "dir"), 0o755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	large := filepath.Join(directory, "large.bin")
	largeFile, err := os.Create(large)
	if err != nil {
		t.Fatalf("creating the large file: %v", err)
	}
	if err := largeFile.Close(); err != nil {
		t.Fatalf("closing the large file: %v", err)
	}
	if err := os.Truncate(large, 300*1024*1024); err != nil {
		t.Fatalf("growing the sparse large file: %v", err)
	}

	cBinary := filepath.Join(directory, "readfatal-c")
	built := runExternal(t, purr, []string{"build", source, "-o", cBinary}, directory, nil)
	if built.status != 0 {
		t.Fatalf("C build: status %d, stderr %q", built.status, built.stderr)
	}
	native := filepath.Join(directory, "readfatal-native")
	written := runExternal(t, executable, []string{"lower", source, native}, directory, nil)
	if written.status != 0 || written.stderr != "" {
		t.Fatalf("native lower: status %d, stderr %q", written.status, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	for _, path := range []string{"data.txt", "empty.txt", "dir", "missing.txt", "large.bin"} {
		t.Run(path, func(t *testing.T) {
			cRun := runExternal(t, cBinary, []string{path}, directory, nil)
			nativeRun := runExternal(t, native, []string{path}, directory, []string{"PATH=/nonexistent"})
			if nativeRun.status != cRun.status || nativeRun.stdout != cRun.stdout || nativeRun.stderr != cRun.stderr {
				t.Fatalf("read_file %s differs:\nC: %d %q %q\nnative: %d %q %q",
					path, cRun.status, cRun.stdout, cRun.stderr,
					nativeRun.status, nativeRun.stdout, nativeRun.stderr)
			}
		})
	}
}

// TestNativeProbeArgv builds the same source through the C and native
// pipelines and runs both with the same non-empty argument vector, including
// an empty argument and one containing spaces. argv[0] differs by executable
// path, so the program prints only the length and arguments 1..n.
func TestNativeProbeArgv(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	purr := goPurrReference(t)
	source := repoPath(t, "..", "..", "tests", "native", "argv.pcy")
	directory := t.TempDir()

	cBinary := filepath.Join(directory, "argv-c")
	built := runExternal(t, purr, []string{"build", source, "-o", cBinary}, directory, nil)
	if built.status != 0 {
		t.Fatalf("C build: status %d, stderr %q", built.status, built.stderr)
	}
	native := filepath.Join(directory, "argv-native")
	written := runExternal(t, executable, []string{"lower", source, native}, directory, nil)
	if written.status != 0 || written.stderr != "" {
		t.Fatalf("native lower: status %d, stderr %q", written.status, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	arguments := []string{"alpha", "beta", "two words", ""}
	cRun := runExternal(t, cBinary, arguments, directory, nil)
	nativeRun := runExternal(t, native, arguments, directory, []string{"PATH=/nonexistent"})
	if nativeRun.status != cRun.status || nativeRun.stdout != cRun.stdout || nativeRun.stderr != cRun.stderr {
		t.Fatalf("native argv differs:\nC: %d %q %q\nnative: %d %q %q",
			cRun.status, cRun.stdout, cRun.stderr, nativeRun.status, nativeRun.stdout, nativeRun.stderr)
	}
}

// TestNativeProbeGetenv builds the environment-reading corpus program through
// both pipelines and runs each with the same controlled environment: a
// present non-empty variable, a present empty variable, an absent variable,
// the empty name, and a name containing `=`. The program prints only shapes
// and lengths, so the comparison covers Set/Unset selection and the copied
// bytes without depending on the host's PATH.
func TestNativeProbeGetenv(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	purr := goPurrReference(t)
	source := repoPath(t, "..", "..", "tests", "native", "getenv.pcy")
	directory := t.TempDir()

	cBinary := filepath.Join(directory, "getenv-c")
	built := runExternal(t, purr, []string{"build", source, "-o", cBinary}, directory, nil)
	if built.status != 0 {
		t.Fatalf("C build: status %d, stderr %q", built.status, built.stderr)
	}
	native := filepath.Join(directory, "getenv-native")
	written := runExternal(t, executable, []string{"lower", source, native}, directory, nil)
	if written.status != 0 || written.stderr != "" {
		t.Fatalf("native lower: status %d, stderr %q", written.status, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	environment := []string{"PATH=/nonexistent", "PCY_GETENV_PROBE_UNSET="}
	cRun := runExternal(t, cBinary, nil, directory, environment)
	nativeRun := runExternal(t, native, nil, directory, environment)
	if nativeRun.status != cRun.status || nativeRun.stdout != cRun.stdout || nativeRun.stderr != cRun.stderr {
		t.Fatalf("native getenv differs:\nC: %d %q %q\nnative: %d %q %q",
			cRun.status, cRun.stdout, cRun.stderr, nativeRun.status, nativeRun.stdout, nativeRun.stderr)
	}
}

// TestNativeProbeLowerRender pins the pooled string-literal scene's
// rendering, byte for byte.
func TestNativeProbeLowerRender(t *testing.T) {
	executable := buildNativeProbe(t)
	source := repoPath(t, "..", "..", "tests", "native", "pool.pcy")
	result := runExternal(t, executable, []string{"lower", source}, t.TempDir(), nil)
	if result.status != 0 || result.stderr != "" {
		t.Fatalf("pool render: status %d, stderr %q", result.status, result.stderr)
	}
	if result.stdout != wantPoolScene {
		t.Fatalf("pool render differs:\n--- got ---\n%s--- want ---\n%s", result.stdout, wantPoolScene)
	}
}

// TestNativeProbeFloatEncode drives the decimal-to-binary64 encoder through
// the probe's `float` mode: every case the corpus carries must equal the
// bit pattern the language-level oracle (binary64 of the same literal)
// produces, across both pipelines' builds. The fixed list pins every edge
// the slice's claims rest on: exact halves and thirds, halfway ties at
// powers of two, the round-trip precision edge, the smallest subnormal, the
// maximal normal's boundary, and the saturations. The seeded random corpus
// stays within the seventeen-digit precision bound; literals beyond it fail
// deterministically and belong to the failure table.
func TestNativeProbeFloatEncode(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()
	cases := []string{
		// Exact dyadic halves at every exponent the bits carry.
		"0.5", "1.5", "2.5", "4.5", "1125899906842624.5", "2251799813685248.5",
		"4503599627370496.5", "9007199254740991.5",
		// Ties where round-to-nearest-even keeps the even side.
		"4503599627370495.5",
		// Thirds and the decimal classics.
		"0.1", "0.2", "0.25", "0.3", "0.75", "3.0625", "3.14159265358979",
		"3.141592653589793", "2.718281828459045", "0.30000000000000001",
		"0.30000000000000004", "0.99999999999999999", "100.5", "512.025",
		// Precision and range edges.
		"9007199254740992.0", "99999999999999999.0", "17976931348623157.0",
		"0.0000000000000000000005", "0.05", "0.005", "0.0005",
		// Saturation and underflow.
		"1" + strings.Repeat("0", 308) + ".0",
		"1" + strings.Repeat("0", 309) + ".0",
		"0" + strings.Repeat("0", 323) + "5.0",
		"0" + strings.Repeat("0", 323) + "25.0",
	}
	rng := rand.New(rand.NewSource(10260030))
	for index := 0; index < 1200; index++ {
		integers := 1 + rng.Intn(5)
		digits := []byte{byte('1' + rng.Intn(9))}
		for position := 1; position < integers; position++ {
			digits = append(digits, byte('0'+rng.Intn(10)))
		}
		fractional := 17 - integers
		if rng.Intn(2) == 0 {
			fractional = 1 + rng.Intn(6)
			if integers+fractional > 17 {
				fractional = 17 - integers
			}
		}
		if fractional < 1 {
			fractional = 1
		}
		for position := 0; position < fractional; position++ {
			digits = append(digits, byte('0'+rng.Intn(10)))
		}
		cases = append(cases, string(digits[:integers])+"."+string(digits[integers:]))
	}
	badCase := "0.300000000000000044"
	failures := 0
	for _, text := range cases {
		result := runExternal(t, executable, []string{"float", text}, directory, nil)
		bits, err := strconv.ParseUint(strings.TrimSpace(result.stdout), 10, 64)
		if err != nil {
			t.Errorf("float %q: probe printed %q", text, result.stdout)
			failures++
			continue
		}
		want := math.Float64bits(mustParseFloat(t, text))
		if uint64(bits) != want {
			t.Errorf("float %q: bits %d, want %d", text, bits, want)
			failures++
		}
	}
	if failures != 0 {
		t.Fatalf("%d float encodings differ from the oracle", failures)
	}
	broken := runExternal(t, executable, []string{"float", badCase}, directory, nil)
	if broken.status != 1 || broken.stdout != "error float literals with more than 17 significant digits are not supported yet\n" {
		t.Fatalf("float %q: status %d, stdout %q", badCase, broken.status, broken.stdout)
	}
}

// mustParseFloat is the language-level oracle: Go's strconv parses a
// decimal literal exactly as correctly as the C toolchain's conversion.
// The range error is the saturation the double performs on out-of-band
// magnitudes, and the returned magnitude (infinite or zero) is what the
// conversion produces, so it is not an oracle disagreement.
func mustParseFloat(t *testing.T, text string) float64 {
	t.Helper()
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		if rangeError, ok := err.(*strconv.NumError); ok && rangeError.Err == strconv.ErrRange {
			return value
		}
		t.Fatalf("the oracle rejects %q: %v", text, err)
	}
	return value
}

// TestNativeProbeLowerFloatRender pins the float scene's rendering, byte
// for byte: bit patterns as constf constants and the SSE2 spellings.
func TestNativeProbeLowerFloatRender(t *testing.T) {
	executable := buildNativeProbe(t)
	source := repoPath(t, "..", "..", "tests", "native", "quarters.pcy")
	result := runExternal(t, executable, []string{"lower", source}, t.TempDir(), nil)
	if result.status != 0 || result.stderr != "" {
		t.Fatalf("quarters render: status %d, stderr %q", result.status, result.stderr)
	}
	if result.stdout != wantQuartersScene {
		t.Fatalf("quarters render differs:\n--- got ---\n%s--- want ---\n%s", result.stdout, wantQuartersScene)
	}
}

// TestNativeProbeLowerIntRender pins the integer-print scene's rendering,
// byte for byte: the allocator shells trail the program's functions, the
// digit scratch stores one byte at a raw offset, and every print(int) call
// reaches "$print_int".
func TestNativeProbeLowerIntRender(t *testing.T) {
	executable := buildNativeProbe(t)
	source := repoPath(t, "..", "..", "tests", "native", "printint.pcy")
	result := runExternal(t, executable, []string{"lower", source}, t.TempDir(), nil)
	if result.status != 0 || result.stderr != "" {
		t.Fatalf("printint render: status %d, stderr %q", result.status, result.stderr)
	}
	if result.stdout != wantPrintIntScene {
		t.Fatalf("printint render differs:\n--- got ---\n%s--- want ---\n%s", result.stdout, wantPrintIntScene)
	}
}

// TestNativeProbeLowerBoolRender pins the bool-print scene's rendering,
// byte for byte: the bool helper branches on the one-byte condition and
// reaches the shared string "$print" routine.
func TestNativeProbeLowerBoolRender(t *testing.T) {
	executable := buildNativeProbe(t)
	source := repoPath(t, "..", "..", "tests", "native", "printbool.pcy")
	result := runExternal(t, executable, []string{"lower", source}, t.TempDir(), nil)
	if result.status != 0 || result.stderr != "" {
		t.Fatalf("printbool render: status %d, stderr %q", result.status, result.stderr)
	}
	if result.stdout != wantPrintBoolScene {
		t.Fatalf("printbool render differs:\n--- got ---\n%s--- want ---\n%s", result.stdout, wantPrintBoolScene)
	}
}

// TestNativeProbeOOM writes the allocation-failure executable with no C
// compiler, runs it with an empty PATH, and demands the C runtime's exact
// pcy_rt_fail spelling for the out-of-memory text: standard error only,
// status 1. A repeated production is byte-identical.
func TestNativeProbeOOM(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()

	native := filepath.Join(directory, "oom-native")
	written := runExternal(t, executable, []string{"elf-oom", native}, directory, nil)
	if written.status != 0 || written.stderr != "" || written.stdout != "built "+native+"\n" {
		t.Fatalf("oom write: status %d, stdout %q, stderr %q", written.status, written.stdout, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	run := runExternal(t, native, nil, directory, []string{"PATH=/nonexistent"})
	if run.status != 1 || run.stderr != "purr: out of memory\n" || run.stdout != "" {
		t.Fatalf("native oom run: status %d, stdout %q, stderr %q", run.status, run.stdout, run.stderr)
	}

	again := filepath.Join(directory, "oom-native-again")
	if written := runExternal(t, executable, []string{"elf-oom", again}, directory, nil); written.status != 0 {
		t.Fatalf("second write status %d", written.status)
	}
	first, err := os.ReadFile(native)
	if err != nil {
		t.Fatalf("reading the first ELF: %v", err)
	}
	secondFile, err := os.ReadFile(again)
	if err != nil {
		t.Fatalf("reading the second ELF: %v", err)
	}
	if string(first) != string(secondFile) {
		t.Fatalf("two oom productions differ in %d bytes", len(first))
	}
}

// TestNativeProbeGC writes the collection-mirror executable with no C
// compiler and runs it with an empty PATH: six hundred one-megabyte
// allocations over the collector, of which only the last reference
// survives, must keep the loop correct through the 256 MiB limit and print
// the close, with status 0. A repeated production is byte-identical.
func TestNativeProbeGC(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()

	native := filepath.Join(directory, "gc-native")
	written := runExternal(t, executable, []string{"elf-gc", native}, directory, nil)
	if written.status != 0 || written.stderr != "" || written.stdout != "built "+native+"\n" {
		t.Fatalf("gc write: status %d, stdout %q, stderr %q", written.status, written.stdout, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	run := runExternal(t, native, nil, directory, []string{"PATH=/nonexistent"})
	if run.status != 0 || run.stderr != "" || run.stdout != "done\n" {
		t.Fatalf("native gc run: status %d, stdout %q, stderr %q", run.status, run.stdout, run.stderr)
	}

	again := filepath.Join(directory, "gc-native-again")
	if written := runExternal(t, executable, []string{"elf-gc", again}, directory, nil); written.status != 0 {
		t.Fatalf("second write status %d", written.status)
	}
	first, err := os.ReadFile(native)
	if err != nil {
		t.Fatalf("reading the first ELF: %v", err)
	}
	secondFile, err := os.ReadFile(again)
	if err != nil {
		t.Fatalf("reading the second ELF: %v", err)
	}
	if string(first) != string(secondFile) {
		t.Fatalf("two gc productions differ in %d bytes", len(first))
	}
}

// TestNativeProbeGCGraph is the white-box transitive-mark proof. The scene
// marks only an outer block, whose payload alone points at an inner block,
// then calls "$scan_heap" directly and reads both ledger bytes. A correct
// pass marks the inner block (2), leaves the unlinked block allocated but
// unmarked (1), reports one newly marked header, and reports zero on the
// second pass. Disabling the heap pass flips the output to
// "missed/unlinked/nochange/settled", so the test is sensitive to the
// transitive edge rather than to conservative stack retention.
func TestNativeProbeGCGraph(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()

	native := filepath.Join(directory, "gc-graph-native")
	written := runExternal(t, executable, []string{"elf-gc-graph", native}, directory, nil)
	if written.status != 0 || written.stderr != "" || written.stdout != "built "+native+"\n" {
		t.Fatalf("gc-graph write: status %d, stdout %q, stderr %q", written.status, written.stdout, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native: %v", err)
	}

	run := runExternal(t, native, nil, directory, []string{"PATH=/nonexistent"})
	if run.status != 0 || run.stderr != "" || run.stdout != "marked\nunlinked\nchanged\nsettled\n" {
		t.Fatalf("native gc-graph run: status %d, stdout %q, stderr %q", run.status, run.stdout, run.stderr)
	}

	again := filepath.Join(directory, "gc-graph-native-again")
	if written := runExternal(t, executable, []string{"elf-gc-graph", again}, directory, nil); written.status != 0 {
		t.Fatalf("second gc-graph write status %d", written.status)
	}
	first, err := os.ReadFile(native)
	if err != nil {
		t.Fatalf("reading the first ELF: %v", err)
	}
	secondFile, err := os.ReadFile(again)
	if err != nil {
		t.Fatalf("reading the second ELF: %v", err)
	}
	if string(first) != string(secondFile) {
		t.Fatalf("two gc-graph productions differ in %d bytes", len(first))
	}
}

// TestNativeProbeAlign runs the white-box stack-alignment scene: three check
// functions capture rsp inside their frames, and main exits with the sum of
// their low nibbles. A correctly aligned selector exits 0; either a
// misaligned frame or an unpadded odd-argument call makes it nonzero.
func TestNativeProbeAlign(t *testing.T) {
	executable := buildNativeProbe(t)
	directory := t.TempDir()
	native := filepath.Join(directory, "align-native")
	written := runExternal(t, executable, []string{"elf-align", native}, directory, nil)
	if written.status != 0 || written.stderr != "" || written.stdout != "built "+native+"\n" {
		t.Fatalf("align write: status %d, stdout %q, stderr %q", written.status, written.stdout, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod align: %v", err)
	}
	run := runExternal(t, native, nil, directory, []string{"PATH=/nonexistent"})
	if run.status != 0 || run.stdout != "" || run.stderr != "" {
		t.Fatalf("align run: status %d, stdout %q, stderr %q (rsp was not 16-byte aligned)", run.status, run.stdout, run.stderr)
	}
	again := filepath.Join(directory, "align-native-again")
	if written := runExternal(t, executable, []string{"elf-align", again}, directory, nil); written.status != 0 {
		t.Fatalf("second align write status %d", written.status)
	}
	first, err := os.ReadFile(native)
	if err != nil {
		t.Fatalf("reading the first align ELF: %v", err)
	}
	second, err := os.ReadFile(again)
	if err != nil {
		t.Fatalf("reading the second align ELF: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("two align productions differ in %d bytes", len(first))
	}
}

// TestNativeProbeNativeFixpoint is the native stage-to-stage proof. The
// C-pipeline probe lowers selfhost/native.pcy into a native stage 2; stage 2
// lowers the same source into stage 3 under an empty PATH; the two ELFs are
// byte-identical. Stage 2 also lowers tests/native/arith.pcy, and that ELF is
// byte-identical to the C-pipeline probe's and runs with the expected output.
func TestNativeProbeNativeFixpoint(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()
	source := repoPath(t, "..", "..", "selfhost", "native.pcy")

	stage2 := filepath.Join(directory, "native-stage2")
	written := runExternal(t, executable, []string{"lower", source, stage2}, directory, nil)
	if written.status != 0 || written.stderr != "" {
		t.Fatalf("stage 2 lower: status %d, stderr %q", written.status, written.stderr)
	}
	if err := os.Chmod(stage2, 0o755); err != nil {
		t.Fatalf("chmod stage 2: %v", err)
	}

	stage3 := filepath.Join(directory, "native-stage3")
	again := runExternal(t, stage2, []string{"lower", source, stage3}, directory, []string{"PATH=/nonexistent"})
	if again.status != 0 || again.stderr != "" {
		t.Fatalf("stage 3 lower: status %d, stderr %q", again.status, again.stderr)
	}
	second, err := os.ReadFile(stage2)
	if err != nil {
		t.Fatalf("reading stage 2: %v", err)
	}
	third, err := os.ReadFile(stage3)
	if err != nil {
		t.Fatalf("reading stage 3: %v", err)
	}
	if string(second) != string(third) {
		t.Fatalf("native stage 2 and stage 3 differ in %d bytes", len(second))
	}

	fixture := repoPath(t, "..", "..", "tests", "native", "arith.pcy")
	cElf := filepath.Join(directory, "arith-c")
	nElf := filepath.Join(directory, "arith-native")
	if written := runExternal(t, executable, []string{"lower", fixture, cElf}, directory, nil); written.status != 0 {
		t.Fatalf("C-pipeline arith lower: status %d, stderr %q", written.status, written.stderr)
	}
	if written := runExternal(t, stage2, []string{"lower", fixture, nElf}, directory, []string{"PATH=/nonexistent"}); written.status != 0 {
		t.Fatalf("native-stage-2 arith lower: status %d, stderr %q", written.status, written.stderr)
	}
	cBytes, err := os.ReadFile(cElf)
	if err != nil {
		t.Fatalf("reading C-pipeline arith ELF: %v", err)
	}
	nBytes, err := os.ReadFile(nElf)
	if err != nil {
		t.Fatalf("reading native-stage-2 arith ELF: %v", err)
	}
	if string(cBytes) != string(nBytes) {
		t.Fatalf("arith ELFs differ in %d bytes", len(cBytes))
	}
	if err := os.Chmod(nElf, 0o755); err != nil {
		t.Fatalf("chmod arith ELF: %v", err)
	}
	run := runExternal(t, nElf, nil, directory, []string{"PATH=/nonexistent"})
	if run.status != 0 || run.stdout != "large\nnegged\n" || run.stderr != "" {
		t.Fatalf("native-stage-2 arith run: status %d, stdout %q, stderr %q", run.status, run.stdout, run.stderr)
	}

	driverSource := repoPath(t, "..", "..", "selfhost", "driver.pcy")
	cDriver := filepath.Join(directory, "driver-c")
	nDriver := filepath.Join(directory, "driver-native")
	if written := runExternal(t, executable, []string{"lower", driverSource, cDriver}, directory, nil); written.status != 0 {
		t.Fatalf("C-pipeline driver lower: status %d, stderr %q", written.status, written.stderr)
	}
	if written := runExternal(t, stage2, []string{"lower", driverSource, nDriver}, directory, []string{"PATH=/nonexistent"}); written.status != 0 {
		t.Fatalf("native-stage-2 driver lower: status %d, stderr %q", written.status, written.stderr)
	}
	cDriverBytes, err := os.ReadFile(cDriver)
	if err != nil {
		t.Fatalf("reading C-pipeline driver: %v", err)
	}
	nDriverBytes, err := os.ReadFile(nDriver)
	if err != nil {
		t.Fatalf("reading native-stage-2 driver: %v", err)
	}
	if string(cDriverBytes) != string(nDriverBytes) {
		t.Fatalf("driver ELFs differ in %d bytes", len(cDriverBytes))
	}
	if err := os.Chmod(nDriver, 0o755); err != nil {
		t.Fatalf("chmod native driver: %v", err)
	}
	stage1 := buildStage1(t)
	fixture = repoPath(t, "..", "..", "tests", "fizzbuzz.pcy")
	reference := runExternal(t, stage1, []string{fixture}, directory, nil)
	nativeStage := runExternal(t, nDriver, []string{fixture}, directory, []string{"PATH=/nonexistent"})
	if nativeStage.status != reference.status || nativeStage.stdout != reference.stdout || nativeStage.stderr != reference.stderr {
		t.Fatalf("native-stage driver C differs:\nreference: %d %q %q\nnative: %d %q %q",
			reference.status, reference.stdout, reference.stderr, nativeStage.status, nativeStage.stdout, nativeStage.stderr)
	}
}

// TestNativeProbeToolchain compiles the production purr toolchain with the
// native backend and runs it: help, a direct run, and a project created by
// new and run in project mode. It proves the native backend compiles the
// whole compiler-facing program, not just the probe and fixtures.
func TestNativeProbeToolchain(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()
	source := repoPath(t, "..", "..", "purr.pcy")

	native := filepath.Join(directory, "purr-native")
	written := runExternal(t, executable, []string{"lower", source, native}, directory, nil)
	if written.status != 0 || written.stderr != "" {
		t.Fatalf("native purr lower: status %d, stderr %q", written.status, written.stderr)
	}
	if err := os.Chmod(native, 0o755); err != nil {
		t.Fatalf("chmod native purr: %v", err)
	}

	help := runExternal(t, native, []string{"help"}, directory, nil)
	if help.status != 0 || !strings.HasPrefix(help.stdout, "Percy compiler\n") {
		t.Fatalf("native purr help: status %d, stdout %q, stderr %q", help.status, help.stdout, help.stderr)
	}

	hello := filepath.Join(directory, "hello.pcy")
	if err := os.WriteFile(hello, []byte("fn main() {\n    print(\"hello from native purr\");\n}\n"), 0o644); err != nil {
		t.Fatalf("writing hello: %v", err)
	}
	run := runExternal(t, native, []string{"run", hello}, directory, nil)
	if run.status != 0 || run.stdout != "hello from native purr\n" || run.stderr != "" {
		t.Fatalf("native purr run: status %d, stdout %q, stderr %q", run.status, run.stdout, run.stderr)
	}

	created := runExternal(t, native, []string{"new", "project"}, directory, nil)
	if created.status != 0 {
		t.Fatalf("native purr new: status %d, stderr %q", created.status, created.stderr)
	}
	project := filepath.Join(directory, "project")
	projectRun := runExternal(t, native, []string{"run"}, project, nil)
	if projectRun.status != 0 || projectRun.stdout != "Hello, world!\n" || projectRun.stderr != "" {
		t.Fatalf("native purr project run: status %d, stdout %q, stderr %q", projectRun.status, projectRun.stdout, projectRun.stderr)
	}
}

// lowerFailureCase is one unsupported-construct source with its expected
// message tail. A positionless case omits the position; the loader's
// read-failure spelling (path with no position) is the pathOnly kind.
type lowerFailureCase struct {
	file    string
	line    string
	col     string
	kind    string // "", "pathless", or "readerror"
	message string
}

func lowerFailureCases(badDirectory string) []lowerFailureCase {
	return []lowerFailureCase{
		{file: "float.pcy", line: "2", col: "21", message: "float literals with more than 17 significant digits are not supported yet"},

		{file: "imported.pcy", line: "1", col: "1", message: "program has no entry point: define fn main() in the root module"},
		{file: "lex.pcy", line: "2", col: "29", message: "newline in string literal"},
		{file: "missing.pcy", kind: "readerror", message: "cannot read source file: open " + filepath.Join(badDirectory, "not_here.pcy") + ": no such file or directory"},
		{file: "parse.pcy", line: "2", col: "17", message: `expected an expression, found Semicolon ";"`},
		{file: "typeerror.pcy", line: "2", col: "17", message: `initializer for variable "value": expected int, found string`},
	}
}

// lowerFailureWant composes the expected `error` line for one case under one
// bad directory spelling.
func lowerFailureWant(badDirectory string, failure lowerFailureCase) string {
	source := filepath.Join(badDirectory, failure.file)
	lead := "error " + source
	if failure.kind == "pathless" {
		return "error " + failure.message + "\n"
	}
	if failure.kind == "readerror" || failure.line == "0" {
		if failure.kind == "readerror" {
			// A read failure names the missing file, not the importer.
			missing := filepath.Join(badDirectory, "not_here.pcy")
			return "error " + missing + ": " + failure.message + "\n"
		}
		return lead + ": " + failure.message + "\n"
	}
	return "error " + source + ":" + failure.line + ":" + failure.col + ": " + failure.message + "\n"
}

// TestNativeProbeLowerFailures pins one deterministic message per
// unsupported construct, lex error, parse error, check error, and load
// failure: on standard output, with status 1 and empty standard error,
// exactly like the failure scenes above.
func TestNativeProbeLowerFailures(t *testing.T) {
	executable := buildNativeProbe(t)
	bad := repoPath(t, "..", "..", "tests", "native", "bad")
	directory := t.TempDir()
	for _, failure := range lowerFailureCases(bad) {
		t.Run(failure.file, func(t *testing.T) {
			source := filepath.Join(bad, failure.file)
			result := runExternal(t, executable, []string{"lower", source}, directory, nil)
			if result.status != 1 {
				t.Fatalf("status %d, want 1", result.status)
			}
			want := lowerFailureWant(filepath.Clean(bad), failure)
			if result.stdout != want {
				t.Fatalf("failure message differs:\n--- got ---\n%s--- want ---\n%s", result.stdout, want)
			}
			if result.stderr != "" {
				t.Fatalf("stderr %q, want empty", result.stderr)
			}
		})
	}
}

// TestNativeProbeLowerFrameLimit lowers an oversized function: the scene is
// valid, so the failure arrives from the selector's frame layout, with the
// C runtime's status behavior intact and no file written.
func TestNativeProbeLowerFrameLimit(t *testing.T) {
	requireToolchain(t)
	executable := buildNativeProbe(t)
	directory := t.TempDir()
	source := filepath.Join(directory, "bigfn.pcy")

	// Each declaration lowers to an initializer temporary plus the local
	// itself, so 4097 declarations produce 8194 locals, just over the
	// selector's 8192-slot frame budget.
	var builder strings.Builder
	builder.WriteString("fn main() {\n")
	for index := 0; index < 4097; index++ {
		fmt.Fprintf(&builder, "    int v%d = 1;\n", index)
	}
	builder.WriteString("    print(v0);\n}\n")
	if err := os.WriteFile(source, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("writing the oversized source: %v", err)
	}

	native := filepath.Join(directory, "big")
	result := runExternal(t, executable, []string{"lower", source, native}, directory, nil)
	if result.status != 1 {
		t.Fatalf("status %d, want 1", result.status)
	}
	if result.stderr != "native: function @main: 8194 locals do not fit the slice's frame layout\n" {
		t.Fatalf("stderr %q", result.stderr)
	}
	if result.stdout != "" {
		t.Fatalf("stdout %q, want empty", result.stdout)
	}
	if _, err := os.Stat(native); !os.IsNotExist(err) {
		t.Fatalf("an oversized function's production exists")
	}
}

// TestNativeProbePercyPipeline builds the probe with the Percy-written
// purr-pcy sidecar, so the typed-IR module also compiles and behaves through
// the Percy pipeline, and compares every probe output against the pinned
// texts the Go pipeline produced.
func TestNativeProbePercyPipeline(t *testing.T) {
	requireToolchain(t)
	stage1 := buildStage1(t)
	percyPurr := buildPurrPcyWithPercy(t, stage1)
	executable := filepath.Join(t.TempDir(), "native-probe-kt")
	result := runPurrPcy(t, percyPurr, t.TempDir(), []string{
		"build", repoPath(t, "..", "..", "selfhost", "native.pcy"), "-o", executable,
	})
	if result.status != 0 || result.stderr != "" || result.stdout != "built "+executable+"\n" {
		t.Fatalf("Percy build of the native probe: status %d, stdout %q, stderr %q",
			result.status, result.stdout, result.stderr)
	}

	goProbe := buildNativeProbe(t)
	cases := []struct {
		mode string
		want string
	}{
		{"", wantHelloScene},
		{"big", "scene\n"},
		{"arith", wantArithScene},
		{"simple", "scene\n"},
		{"no-label", "error function @main: instruction 0: label 0 is out of range (the function defines 0 labels)\n"},
		{"bad-binding", "error function @main: instruction 0: target binding 0 is out of range (the function has 0 bindings)\n"},
		{"bad-call-count", "error function @main: instruction 0: the call passes 0 arguments, but @print takes 1\n"},
		{"bad-call-shape", "error function @main: instruction 0: argument 0 binding 0 has bool, expected ref\n"},
		{"bad-entry", "error entry 5 is out of range (the scene has 1 functions)\n"},
		{"undefined-label", "error function @main: instruction 1: true label 1 is out of range (the function defines 1 labels)\n"},
		{"wat", "error entry 0 is out of range (the scene has 0 functions)\n"},
		{"bad-static-blob", "error function @main: instruction 0: blob 9 is out of range (the scene has 1 blobs)\n"},
		{"bad-static-offset", "error function @main: instruction 0: offset 60 does not fit blob $blob0's 32 bytes\n"},
		{"bad-static-value", "error function @main: instruction 0: value binding 0 has bool, expected int\n"},
	}
	for _, testCase := range cases {
		arguments := []string{}
		if testCase.mode != "" {
			arguments = append(arguments, testCase.mode)
		}
		goResult := runExternal(t, goProbe, arguments, t.TempDir(), nil)
		ktResult := runExternal(t, executable, arguments, t.TempDir(), nil)
		if ktResult.status != goResult.status ||
			ktResult.stdout != goResult.stdout ||
			ktResult.stderr != goResult.stderr {
			t.Errorf("mode %q: Percy %d/%q/%q, Go %d/%q/%q",
				testCase.mode, ktResult.status, ktResult.stdout, ktResult.stderr,
				goResult.status, goResult.stdout, goResult.stderr)
		}
	}
}

// TestNativeProbePercyPipelineSkeleton builds the probe with the
// Percy-written sidecar as well, and the ELFs both pipelines produce must
// be byte-identical for every scene kind: the same selector and ELF writer
// run through the Percy backend. The hello executable's behavior is
// asserted directly; arith and divzero already are in their own tests.
func TestNativeProbePercyPipelineSkeleton(t *testing.T) {
	requireToolchain(t)
	stage1 := buildStage1(t)
	percyPurr := buildPurrPcyWithPercy(t, stage1)
	ktExecutable := filepath.Join(t.TempDir(), "native-probe-kt")
	result := runPurrPcy(t, percyPurr, t.TempDir(), []string{
		"build", repoPath(t, "..", "..", "selfhost", "native.pcy"), "-o", ktExecutable,
	})
	if result.status != 0 || result.stderr != "" || result.stdout != "built "+ktExecutable+"\n" {
		t.Fatalf("Percy build of the native probe: status %d, stdout %q, stderr %q",
			result.status, result.stdout, result.stderr)
	}

	goExecutable := buildNativeProbe(t)
	directory := t.TempDir()

	// One ELF per scene kind: unadorned "elf" lowers hello, and the two
	// Phase 10.3 scene names lower their own executable.
	sceneKinds := []struct {
		mode string
		name string
	}{
		{"elf", "hello"},
		{"elf-arith", "arith"},
		{"elf-divzero", "divzero"},
		{"elf-oom", "oom"},
		{"elf-gc", "gc"},
	}
	for _, kind := range sceneKinds {
		goNative := filepath.Join(directory, kind.name+"-go")
		if written := runExternal(t, goExecutable, []string{kind.mode, goNative}, directory, nil); written.status != 0 {
			t.Fatalf("Go pipeline %s elf: status %d, stderr %q", kind.name, written.status, written.stderr)
		}
		ktNative := filepath.Join(directory, kind.name+"-kt")
		if written := runExternal(t, ktExecutable, []string{kind.mode, ktNative}, directory, nil); written.status != 0 {
			t.Fatalf("Percy pipeline %s elf: status %d, stderr %q", kind.name, written.status, written.stderr)
		}
		goBytes, err := os.ReadFile(goNative)
		if err != nil {
			t.Fatalf("reading the Go-produced %s ELF: %v", kind.name, err)
		}
		ktBytes, err := os.ReadFile(ktNative)
		if err != nil {
			t.Fatalf("reading the Percy-produced %s ELF: %v", kind.name, err)
		}
		if string(goBytes) != string(ktBytes) {
			t.Fatalf("the two pipelines' %s ELFs differ in %d bytes", kind.name, len(goBytes))
		}
		// The hello executable's behavior is asserted directly; arith and
		// divzero already are in their own tests.
		if kind.name == "hello" {
			if err := os.Chmod(ktNative, 0o755); err != nil {
				t.Fatalf("chmod the Percy-produced ELF: %v", err)
			}
			run := runExternal(t, ktNative, nil, directory, nil)
			if run.status != 0 || run.stdout != "hello\n" || run.stderr != "" {
				t.Fatalf("Percy-produced native run: %d %q %q", run.status, run.stdout, run.stderr)
			}
		}
	}
}

// TestNativeProbePercyPipelineLower builds the probe with the Percy-written
// purr-pcy sidecar as well and opposes the two pipelines over the whole
// lowering corpus: every scene rendering and every lowering failure message
// must agree byte for byte, and each corpus program's ELF must be
// byte-identical. The Percy-produced arithmetic executable's behavior is
// asserted directly against the C pipeline's build; the rest share the ELF
// bytes exactly. The bad corpus lives in one directory used for every run,
// so the canonical paths inside message texts agree too.
func TestNativeProbePercyPipelineLower(t *testing.T) {
	requireToolchain(t)
	stage1 := buildStage1(t)
	percyPurr := buildPurrPcyWithPercy(t, stage1)
	ktExecutable := filepath.Join(t.TempDir(), "native-probe-kt")
	built := runPurrPcy(t, percyPurr, t.TempDir(), []string{
		"build", repoPath(t, "..", "..", "selfhost", "native.pcy"), "-o", ktExecutable,
	})
	if built.status != 0 || built.stderr != "" || built.stdout != "built "+ktExecutable+"\n" {
		t.Fatalf("Percy build of the native probe: status %d, stdout %q, stderr %q",
			built.status, built.stdout, built.stderr)
	}
	goExecutable := buildNativeProbe(t)

	// One directory for every run, so message texts carrying paths agree.
	directory := t.TempDir()
	sources := lowerCorpusSources(t)
	sources = append(sources, repoPath(t, "..", "..", "tests", "modules", "main.pcy"))

	for _, source := range sources {
		name := filepath.Base(source)
		t.Run(name, func(t *testing.T) {
			goRender := runExternal(t, goExecutable, []string{"lower", source}, directory, nil)
			ktRender := runExternal(t, ktExecutable, []string{"lower", source}, directory, nil)
			if ktRender.status != goRender.status || ktRender.stdout != goRender.stdout || ktRender.stderr != goRender.stderr {
				t.Fatalf("renders differ:\nGo: %d %q %q\nPercy: %d %q %q",
					goRender.status, goRender.stdout, goRender.stderr,
					ktRender.status, ktRender.stdout, ktRender.stderr)
			}

			goNative := filepath.Join(directory, name+"-go")
			pthread := filepath.Join(directory, name+"-kt")
			if written := runExternal(t, goExecutable, []string{"lower", source, goNative}, directory, nil); written.status != 0 {
				t.Fatalf("Go pipeline lower: status %d, stderr %q", written.status, written.stderr)
			}
			if written := runExternal(t, ktExecutable, []string{"lower", source, pthread}, directory, nil); written.status != 0 {
				t.Fatalf("Percy pipeline lower: status %d, stderr %q", written.status, written.stderr)
			}
			goBytes, err := os.ReadFile(goNative)
			if err != nil {
				t.Fatalf("reading the Go-produced ELF: %v", err)
			}
			ktBytes, err := os.ReadFile(pthread)
			if err != nil {
				t.Fatalf("reading the Percy-produced ELF: %v", err)
			}
			if string(goBytes) != string(ktBytes) {
				t.Fatalf("the two pipelines' %s ELFs differ in %d bytes", name, len(goBytes))
			}

			if name == "arith.pcy" {
				if err := os.Chmod(pthread, 0o755); err != nil {
					t.Fatalf("chmod the Percy-produced ELF: %v", err)
				}
				run := runExternal(t, pthread, nil, directory, []string{"PATH=/nonexistent"})
				if run.status != 0 || run.stdout != "large\nnegged\n" || run.stderr != "" {
					t.Fatalf("Percy-produced arith run: %d %q %q", run.status, run.stdout, run.stderr)
				}
			}
		})
	}

	bad := filepath.Clean(repoPath(t, "..", "..", "tests", "native", "bad"))
	for _, failure := range lowerFailureCases(bad) {
		t.Run("bad-"+failure.file, func(t *testing.T) {
			source := filepath.Join(bad, failure.file)
			goResult := runExternal(t, goExecutable, []string{"lower", source}, directory, nil)
			ktResult := runExternal(t, ktExecutable, []string{"lower", source}, directory, nil)
			if ktResult.status != goResult.status || ktResult.stdout != goResult.stdout || ktResult.stderr != goResult.stderr {
				t.Fatalf("failures differ for %s:\nGo: %d %q %q\nPercy: %d %q %q",
					failure.file, goResult.status, goResult.stdout, goResult.stderr,
					ktResult.status, ktResult.stdout, ktResult.stderr)
			}
		})
	}
}
