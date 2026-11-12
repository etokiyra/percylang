package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"percy/checker"
	"percy/codegen"
	"percy/lexer"
	"percy/loader"
	"percy/parser"
)

func runtimeMemorySource(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "runtime", "pcy", "memory.pcy")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func runtimeModules(t *testing.T, source string) ([]*parser.Module, *checker.Result) {
	t.Helper()
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lexing runtime test source: %v", err)
	}
	program, err := parser.Parse(tokens)
	if err != nil {
		t.Fatalf("parsing runtime test source: %v", err)
	}
	modules := []*parser.Module{
		{Path: "input.txt", ID: 0, Program: program},
		{Path: "input2.txt", ID: 1, Program: &parser.Program{}},
	}
	checked, err := checker.CheckModulesWithRuntime(modules, runtimeMemorySource(t))
	if err != nil {
		t.Fatalf("checking runtime test source: %v", err)
	}
	return modules, checked
}

func runPercyWithRuntime(t *testing.T, executable, runtimeSource, root, imported string) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "runtime.txt"), []byte(runtimeSource), 0o644); err != nil {
		t.Fatalf("writing runtime.txt: %v", err)
	}
	output, err := runPercyUnit(executable, directory, root, imported)
	if err != nil {
		t.Fatalf("running Percy probe with runtime: %v", err)
	}
	return output
}

func TestRuntimeModuleMemoryHelper(t *testing.T) {
	requireToolchain(t)
	source := `fn main() {
    byte[] data = new byte[3];
    data[0] = byte_from_int(65);
    data[1] = byte_from_int(66);
    data[2] = byte_from_int(67);
    byte[] copy = copy_bytes(data);
    print(length(copy), int_from_byte(copy[0]), int_from_byte(copy[2]));
    zero_bytes(copy, 3);
    print(int_from_byte(copy[0]), int_from_byte(copy[1]), int_from_byte(copy[2]));
}
`
	modules, checked := runtimeModules(t, source)
	if len(checked.Modules) != 3 {
		t.Fatalf("module count = %d, want root, import, and runtime", len(checked.Modules))
	}
	if checked.Modules[2].Path != runtimeModulePathForTest() || checked.Modules[2].ID != 2 {
		t.Errorf("runtime module = %q id %d", checked.Modules[1].Path, checked.Modules[1].ID)
	}
	generated, err := codegen.Generate(modules, checked)
	if err != nil {
		t.Fatalf("generating C: %v", err)
	}
	if !strings.Contains(generated, "pcy_rt_raw_copy") || !strings.Contains(generated, "copy_bytes") {
		t.Fatalf("generated C does not contain the runtime helper and intrinsic:\n%s", generated)
	}

	unitExecutable := buildPercyUnit(t)
	percyText := runPercyWithRuntime(t, unitExecutable, runtimeMemorySource(t), source, "")
	if difference := unitDifference(percyText, generated); difference != "" {
		t.Fatalf("runtime translation unit differs: %s", difference)
	}
	compiler, err := discoverCompiler()
	if err != nil {
		t.Fatalf("discovering compiler: %v", err)
	}
	compileFlags, linkFlags := gcBuildFlags()
	result, err := buildAndRunProgramWithArgs(t, compiler, compileFlags, linkFlags, t.TempDir(), generated, 0, nil)
	if err != nil {
		t.Fatalf("compiling and running runtime program: %v", err)
	}
	if result.status != 0 || result.stderr != "" || result.stdout != "3 65 67\n0 0 0\n" {
		t.Errorf("runtime program: stdout %q, stderr %q, status %d", result.stdout, result.stderr, result.status)
	}
}

func TestRuntimeModuleWithTempDirectory(t *testing.T) {
	requireToolchain(t)
	source := `fn main() {
    try_create_temp_directory("purr");
    copy_bytes(new byte[0]);
}
`
	modules, checked := runtimeModules(t, source)
	if len(checked.Modules) != 4 {
		t.Fatalf("module count = %d, want root, import, prelude, and runtime", len(checked.Modules))
	}
	if checked.Modules[2].Path != "<prelude>" || checked.Modules[2].ID != 2 {
		t.Errorf("prelude = %q id %d", checked.Modules[2].Path, checked.Modules[2].ID)
	}
	if checked.Modules[3].Path != runtimeModulePathForTest() || checked.Modules[3].ID != 3 {
		t.Errorf("runtime = %q id %d", checked.Modules[3].Path, checked.Modules[3].ID)
	}
	generated, err := codegen.Generate(modules, checked)
	if err != nil {
		t.Fatalf("generating C: %v", err)
	}
	unitExecutable := buildPercyUnit(t)
	percyText := runPercyWithRuntime(t, unitExecutable, runtimeMemorySource(t), source, "")
	if difference := unitDifference(percyText, generated); difference != "" {
		t.Fatalf("runtime translation unit differs: %s", difference)
	}
}

func TestRuntimeModuleModuleIDs(t *testing.T) {
	source := `fn main() {
    try_read_file("x");
    try_create_temp_directory("purr");
    copy_bytes(new byte[0]);
}
`
	modules, checked := runtimeModules(t, source)
	if len(checked.Modules) != 4 {
		t.Fatalf("module count = %d, want root, import, prelude, and runtime", len(checked.Modules))
	}
	if checked.Modules[2].Path != "<prelude>" || checked.Modules[2].ID != 2 {
		t.Errorf("prelude = %q id %d", checked.Modules[2].Path, checked.Modules[2].ID)
	}
	if checked.Modules[3].Path != "<runtime>" || checked.Modules[3].ID != 3 {
		t.Errorf("runtime = %q id %d", checked.Modules[2].Path, checked.Modules[2].ID)
	}
	_ = modules
}

func TestRuntimeModuleIntrinsics(t *testing.T) {
	requireToolchain(t)
	runtimeSource := runtimeMemorySource(t) + `
public fn exercise(byte[] data, string text) -> int {
    raw_store8(address_of(data), byte_from_int(65));
    int first = int_from_byte(raw_load8(address_of(data)));
    raw_store64(address_of(data) + 1, 123456789);
    int second = raw_load64(address_of(data) + 1);
    if (second != 123456789) { return -1; }
    raw_copy(address_of(data) + 2, address_of(data), 2);
    raw_zero(address_of(data) + 2, 1);
    return first + int_from_byte(raw_load8(address_of(data) + 2)) + int_from_byte(raw_load8(address_of(text)));
}
public fn getpid_raw() -> int {
    return syscall(39, 0, 0, 0, 0, 0, 0);
}
public fn write_stderr(string text) -> int {
    return syscall(1, 2, address_of(text), length(text), 0, 0, 0);
}
`
	source := `fn main() {
    byte[] data = new byte[16];
    print(exercise(data, "A"));
    print(getpid_raw());
    write_stderr("syscall-write\n");
}
`
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lexing: %v", err)
	}
	program, err := parser.Parse(tokens)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	modules := []*parser.Module{
		{Path: "input.txt", ID: 0, Program: program},
		{Path: "input2.txt", ID: 1, Program: &parser.Program{}},
	}
	checked, err := checker.CheckModulesWithRuntime(modules, runtimeSource)
	if err != nil {
		t.Fatalf("checking intrinsics: %v", err)
	}
	generated, err := codegen.Generate(modules, checked)
	if err != nil {
		t.Fatalf("generating intrinsics C: %v", err)
	}
	unitExecutable := buildPercyUnit(t)
	percyText := runPercyWithRuntime(t, unitExecutable, runtimeSource, source, "")
	if difference := unitDifference(percyText, generated); difference != "" {
		t.Fatalf("intrinsics translation unit differs: %s", difference)
	}
	compiler, err := discoverCompiler()
	if err != nil {
		t.Fatalf("discovering compiler: %v", err)
	}
	compileFlags, linkFlags := gcBuildFlags()
	executable, err := buildPercyRunProgram(t, compiler, compileFlags, linkFlags, generated)
	if err != nil {
		t.Fatalf("%v", err)
	}
	command := exec.Command(executable)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("starting intrinsics program: %v", err)
	}
	pid := command.Process.Pid
	if err := command.Wait(); err != nil {
		t.Fatalf("running intrinsics program: %v (stderr %s)", err, stderr.String())
	}
	want := fmt.Sprintf("130\n%d\n", pid)
	if stdout.String() != want || stderr.String() != "syscall-write\n" {
		t.Errorf("intrinsics program: stdout %q, stderr %q; want %q, %q", stdout.String(), stderr.String(), want, "syscall-write\n")
	}
}

func TestRuntimeIntrinsicVisibilityAndErrors(t *testing.T) {
	requireToolchain(t)
	userSource := `fn main() {
    print(int_from_byte(raw_load8(0)));
}
`
	// Without the hook, the name is an ordinary undefined function.
	if _, err := checkSingleSource(t, userSource); err == nil || !strings.Contains(err.Error(), `undefined function "raw_load8"`) {
		t.Errorf("raw_load8 without hook: %v", err)
	}
	// Even with the runtime module, intrinsic names do not resolve in user code.
	if _, err := checker.CheckModulesWithRuntime([]*parser.Module{parseTestModuleForRuntime(t, userSource)}, runtimeMemorySource(t)); err == nil || !strings.Contains(err.Error(), `undefined function "raw_load8"`) {
		t.Errorf("raw_load8 with hook in user module: %v", err)
	}
	// A user module may declare the name as an ordinary function.
	ordinary := `fn raw_load8(int value) -> int {
    return value;
}
fn main() {
    print(raw_load8(7));
}
`
	if _, err := checkSingleSource(t, ordinary); err != nil {
		t.Errorf("ordinary raw_load8 declaration rejected: %v", err)
	}
	// Wrong arity and wrong address_of type are runtime-module errors.
	badArity := `public fn bad() {
    raw_load8();
}
`
	if _, err := checker.CheckModulesWithRuntime([]*parser.Module{parseTestModuleForRuntime(t, "fn main() {}\n")}, badArity); err == nil || !strings.Contains(err.Error(), "raw_load8") {
		t.Errorf("wrong intrinsic arity: %v", err)
	}
	badAddress := `public fn bad() {
    address_of(1);
}
`
	if _, err := checker.CheckModulesWithRuntime([]*parser.Module{parseTestModuleForRuntime(t, "fn main() {}\n")}, badAddress); err == nil || !strings.Contains(err.Error(), "expected byte[] or string") {
		t.Errorf("wrong address_of type: %v", err)
	}
}

func TestRuntimeIntrinsicPercyErrors(t *testing.T) {
	requireToolchain(t)
	checkerExecutable := buildPercyChecker(t)
	userSource := "fn main() {\n    print(int_from_byte(raw_load8(0)));\n}\n"
	wantNoHook, err := renderReferenceCheck(userSource, "")
	if err != nil {
		t.Fatalf("rendering no-hook reference: %v", err)
	}
	noHookOutput, err := runPercyChecker(checkerExecutable, t.TempDir(), userSource, "")
	if err != nil {
		t.Fatalf("running Percy checker without hook: %v", err)
	}
	if noHookOutput != wantNoHook {
		t.Errorf("Percy no-hook raw_load8: got %q want %q", noHookOutput, wantNoHook)
	}
	wantWithHook, err := renderReferenceCheckWithRuntime(userSource, "", runtimeMemorySource(t))
	if err != nil {
		t.Fatalf("rendering hooked reference: %v", err)
	}
	if got := runPercyCheckerWithRuntime(t, checkerExecutable, runtimeMemorySource(t), userSource, ""); got != wantWithHook {
		t.Errorf("Percy hooked user raw_load8: got %q want %q", got, wantWithHook)
	}
	badRuntime := "public fn bad() {\n    raw_load8();\n}\n"
	wantBad, err := renderReferenceCheckWithRuntime("fn main() {}\n", "", badRuntime)
	if err != nil {
		t.Fatalf("rendering bad-runtime reference: %v", err)
	}
	if got := runPercyCheckerWithRuntime(t, checkerExecutable, badRuntime, "fn main() {}\n", ""); got != wantBad {
		t.Errorf("Percy bad runtime arity: got %q want %q", got, wantBad)
	}
}

func TestImportRuntimeModuleFails(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "main.pcy")
	if err := os.WriteFile(source, []byte("import \"<runtime>\";\nfn main() {}\n"), 0o644); err != nil {
		t.Fatalf("writing import fixture: %v", err)
	}
	if _, err := loader.Load(source); err == nil {
		t.Fatalf("import \"<runtime>\" succeeded, want a missing-module error")
	}
}

func runtimeModulePathForTest() string {
	return "<runtime>"
}

func parseTestModuleForRuntime(t *testing.T, source string) *parser.Module {
	t.Helper()
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lexing: %v", err)
	}
	program, err := parser.Parse(tokens)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	return &parser.Module{Path: "input.txt", ID: 0, Program: program}
}

func checkSingleSource(t *testing.T, source string) (*checker.Result, error) {
	t.Helper()
	return checker.CheckModules([]*parser.Module{parseTestModuleForRuntime(t, source)})
}

var _ = errors.New

func runPercyCheckerWithRuntime(t *testing.T, executable, runtimeSource, root, imported string) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "runtime.txt"), []byte(runtimeSource), 0o644); err != nil {
		t.Fatalf("writing runtime.txt: %v", err)
	}
	output, err := runPercyChecker(executable, directory, root, imported)
	if err != nil {
		t.Fatalf("running Percy checker with runtime: %v", err)
	}
	return output
}

func runPercySemanticsWithRuntime(t *testing.T, executable, runtimeSource, root, imported string) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "runtime.txt"), []byte(runtimeSource), 0o644); err != nil {
		t.Fatalf("writing runtime.txt: %v", err)
	}
	output, err := runPercySemantics(executable, directory, root, imported)
	if err != nil {
		t.Fatalf("running Percy semantics with runtime: %v", err)
	}
	return output
}

func TestRuntimeModuleCheckerAndSemantics(t *testing.T) {
	requireToolchain(t)
	source := `fn main() {
    byte[] data = new byte[2];
    byte[] copy = copy_bytes(data);
    print(length(copy));
}
`
	runtimeSource := runtimeMemorySource(t)
	wantCheck, err := renderReferenceCheckWithRuntime(source, "", runtimeSource)
	if err != nil {
		t.Fatalf("rendering reference checker result: %v", err)
	}
	gotCheck := runPercyCheckerWithRuntime(t, buildPercyChecker(t), runtimeSource, source, "")
	if gotCheck != wantCheck {
		t.Errorf("runtime checker output:\n got %q\nwant %q", gotCheck, wantCheck)
	}
	if !strings.Contains(wantCheck, "ok modules 3 ") {
		t.Errorf("runtime checker output does not report three modules: %q", wantCheck)
	}

	wantModel, err := referenceModelWithRuntime(source, "", runtimeSource)
	if err != nil {
		t.Fatalf("rendering reference semantic model: %v", err)
	}
	gotModel := runPercySemanticsWithRuntime(t, buildPercySemantics(t), runtimeSource, source, "")
	if difference := unitDifference(gotModel, wantModel); difference != "" {
		t.Errorf("runtime semantic model differs: %s", difference)
	}
}

func TestRuntimeNegativeSize(t *testing.T) {
	requireToolchain(t)
	runtimeSource := runtimeMemorySource(t) + `
public fn fail_zero(byte[] data) {
    raw_zero(address_of(data), -1);
}
`
	source := `fn main() {
    print("before");
    fail_zero(new byte[1]);
}
`
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lexing: %v", err)
	}
	program, err := parser.Parse(tokens)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	modules := []*parser.Module{
		{Path: "input.txt", ID: 0, Program: program},
		{Path: "input2.txt", ID: 1, Program: &parser.Program{}},
	}
	checked, err := checker.CheckModulesWithRuntime(modules, runtimeSource)
	if err != nil {
		t.Fatalf("checking negative-size fixture: %v", err)
	}
	generated, err := codegen.Generate(modules, checked)
	if err != nil {
		t.Fatalf("generating negative-size C: %v", err)
	}
	percyText := runPercyWithRuntime(t, buildPercyUnit(t), runtimeSource, source, "")
	if difference := unitDifference(percyText, generated); difference != "" {
		t.Fatalf("negative-size translation unit differs: %s", difference)
	}
	compiler, err := discoverCompiler()
	if err != nil {
		t.Fatalf("discovering compiler: %v", err)
	}
	compileFlags, linkFlags := gcBuildFlags()
	result, err := buildAndRunProgramWithArgs(t, compiler, compileFlags, linkFlags, t.TempDir(), generated, 0, nil)
	if err != nil {
		t.Fatalf("compiling negative-size program: %v", err)
	}
	if result.status != 1 || result.stdout != "before\n" || result.stderr != "purr: negative size -1\n" {
		t.Errorf("negative-size program: stdout %q, stderr %q, status %d", result.stdout, result.stderr, result.status)
	}
}

func TestRuntimeModuleSanitizer(t *testing.T) {
	requireToolchain(t)
	source := `fn main() {
    byte[] data = new byte[16];
    raw_store64(address_of(data) + 1, 42);
    print(raw_load64(address_of(data) + 1));
    raw_copy(address_of(data) + 2, address_of(data), 4);
    raw_zero(address_of(data) + 3, 2);
}
`
	// The user source above cannot resolve intrinsics; use a runtime helper.
	source = `fn main() {
    exercise_words(new byte[16]);
}
`
	runtimeSource := runtimeMemorySource(t) + `
public fn exercise_words(byte[] data) {
    raw_store64(address_of(data) + 1, 42);
    print(raw_load64(address_of(data) + 1));
    raw_copy(address_of(data) + 2, address_of(data), 4);
    raw_zero(address_of(data) + 3, 2);
}
`
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lexing sanitizer source: %v", err)
	}
	program, err := parser.Parse(tokens)
	if err != nil {
		t.Fatalf("parsing sanitizer source: %v", err)
	}
	modules := []*parser.Module{
		{Path: "input.txt", ID: 0, Program: program},
		{Path: "input2.txt", ID: 1, Program: &parser.Program{}},
	}
	checked, err := checker.CheckModulesWithRuntime(modules, runtimeSource)
	if err != nil {
		t.Fatalf("checking sanitizer source: %v", err)
	}
	generated, err := codegen.Generate(modules, checked)
	if err != nil {
		t.Fatalf("generating sanitizer C: %v", err)
	}
	compiler, err := discoverCompiler()
	if err != nil {
		t.Fatalf("discovering compiler: %v", err)
	}
	compileFlags, linkFlags := gcBuildFlags()
	compileFlags = append(compileFlags, "-fsanitize=undefined")
	result, err := buildAndRunProgramWithArgs(t, compiler, compileFlags, linkFlags, t.TempDir(), generated, 0, nil)
	if err != nil {
		t.Skipf("undefined behavior sanitizer is unavailable with %s: %v", filepath.Base(compiler[0]), err)
	}
	if result.status != 0 || result.stderr != "" || result.stdout != "42\n" {
		t.Errorf("sanitizer program: stdout %q, stderr %q, status %d", result.stdout, result.stderr, result.status)
	}
}

func TestRuntimeModuleWithPreludePercy(t *testing.T) {
	requireToolchain(t)
	source := `fn main() {
    try_read_file("x");
    copy_bytes(new byte[0]);
}
`
	runtimeSource := runtimeMemorySource(t)
	want, err := renderReferenceCheckWithRuntime(source, "", runtimeSource)
	if err != nil {
		t.Fatalf("rendering runtime+prelude reference: %v", err)
	}
	got := runPercyCheckerWithRuntime(t, buildPercyChecker(t), runtimeSource, source, "")
	if got != want {
		t.Errorf("Percy runtime+prelude checker output:\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(want, "ok modules 4 ") {
		t.Errorf("runtime+prelude module count is not four: %q", want)
	}
}

// TestRuntimeIoWriterBothPipelines checks the reserved runtime/pcy module and
// its syscall-backed executable writer through the Go C pipeline and the
// native backend. The file must exist, be executable, and hold the bytes.
func TestRuntimeIoWriterBothPipelines(t *testing.T) {
	requireToolchain(t)
	goPurr := goPurrReference(t)
	nativeProbe := buildNativeProbe(t)
	directory := t.TempDir()
	runtimeSource, err := os.ReadFile(repoPath(t, "..", "..", "runtime", "pcy", "io.pcy"))
	if err != nil {
		t.Fatalf("reading runtime io source: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "runtime", "pcy"), 0o755); err != nil {
		t.Fatalf("creating runtime directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "runtime", "pcy", "io.pcy"), runtimeSource, 0o644); err != nil {
		t.Fatalf("writing runtime io source: %v", err)
	}
	source := `import "runtime/pcy/io.pcy";
fn main() {
    byte[] data = new byte[2];
    data[0] = byte_from_int(65);
    data[1] = byte_from_int(66);
    int result = write_executable("out.bin", data);
    print("write", result);
    match (try_file_info("out.bin")) {
        Ok(is_file, is_directory, is_symlink, executable, size) { print(is_file, executable, size); }
        Missing { print("missing"); }
        Failed(message) { print(message); }
    }
}
`
	if err := os.WriteFile(filepath.Join(directory, "probe.pcy"), []byte(source), 0o644); err != nil {
		t.Fatalf("writing probe: %v", err)
	}
	want := "write 0\ntrue true 2\n"

	goRun := runExternal(t, goPurr, []string{"run", "probe.pcy"}, directory, nil)
	if goRun.status != 0 || goRun.stderr != "" || goRun.stdout != want {
		t.Fatalf("Go C pipeline: status %d, stdout %q, stderr %q", goRun.status, goRun.stdout, goRun.stderr)
	}

	elf := filepath.Join(directory, "probe-native")
	lowered := runExternal(t, nativeProbe, []string{"lower", "probe.pcy", elf}, directory, nil)
	if lowered.status != 0 || lowered.stderr != "" {
		t.Fatalf("native lower: status %d, stderr %q", lowered.status, lowered.stderr)
	}
	if err := os.Chmod(elf, 0o755); err != nil {
		t.Fatalf("chmod native probe: %v", err)
	}
	nativeRun := runExternal(t, elf, nil, directory, []string{"PATH=/nonexistent"})
	if nativeRun.status != 0 || nativeRun.stderr != "" || nativeRun.stdout != want {
		t.Fatalf("native pipeline: status %d, stdout %q, stderr %q", nativeRun.status, nativeRun.stdout, nativeRun.stderr)
	}
}
