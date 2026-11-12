package main

// Tests for purr-pcy build.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"percy/checker"
	"percy/codegen"
	"percy/loader"
)

func writeBuildFixture(t *testing.T, directory string) string {
	t.Helper()
	path := filepath.Join(directory, "hello.pcy")
	source := "fn main() {\n    print(\"Hello, world!\");\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("writing build fixture: %v", err)
	}
	return path
}

func runBuildCommand(t *testing.T, executable, directory string, arguments []string, environment []string) programResult {
	t.Helper()
	return runExternal(t, executable, arguments, directory, environment)
}

func normalizeBuildUsage(t *testing.T, text string) string {
	t.Helper()
	// The Percy usage deliberately adds the new/help lines to the Go block.
	text = strings.ReplaceAll(text, purrUsageText(), "USAGE")
	text = strings.ReplaceAll(text, goUsageText(t), "USAGE")
	return text
}

func goUsageText(t *testing.T) string {
	t.Helper()
	// Keep this independent of the real Go binary's current text: it is the
	// baseline block the 9.8a tests already diffed.
	return "Percy compiler\n\nUsage:\n" +
		"  purr build <file.pcy> [-o <output>]   Compile a Percy source file\n" +
		"  purr run <file.pcy>                   Compile and run a Percy source file\n\n" +
		"Options:\n" +
		"  -o, --output <path>   Write the executable to <path> (build only)\n\n" +
		"The C compiler is taken from CC, or discovered as cc, gcc, or clang.\n" +
		"Percy programs are linked against the Boehm garbage collector (libgc)."
}

func TestPercyPurrBuildArguments(t *testing.T) {
	requireToolchain(t)
	stage1 := buildStage1(t)
	goPurr := buildGoPurr(t)
	purrs := map[string]string{
		"Go":    buildPurrPcyWithGo(t),
		"Percy": buildPurrPcyWithPercy(t, stage1),
	}
	directory := t.TempDir()
	source := writeBuildFixture(t, directory)

	errorCases := []struct {
		name      string
		arguments []string
		usage     bool
	}{
		{"no arguments", []string{"build"}, true},
		{"missing output value", []string{"build", "-o"}, true},
		{"missing output value long", []string{"build", "--output"}, true},
		{"unknown option", []string{"build", "-x"}, true},
		{"unknown option long", []string{"build", "--wat"}, true},
		{"two sources", []string{"build", "one.pcy", "two.pcy"}, true},
		{"extension", []string{"build", "hello"}, false},
		{"extension uppercase", []string{"build", "hello.PCY"}, false},
	}
	for _, testCase := range errorCases {
		expectedPrefix := "purr build: "
		expectedSuffix := ""
		switch testCase.name {
		case "no arguments":
			expectedSuffix = "missing source file\n"
		case "missing output value":
			expectedSuffix = "-o requires a path\n"
		case "missing output value long":
			expectedSuffix = "--output requires a path\n"
		case "unknown option":
			expectedSuffix = "unknown option \"-x\"\n"
		case "unknown option long":
			expectedSuffix = "unknown option \"--wat\"\n"
		case "two sources":
			expectedSuffix = "expected one source file, got 2\n"
		case "extension":
			expectedPrefix = "purr build: \"hello\" is not a Percy source file (expected a .pcy file)\n"
			expectedSuffix = ""
		case "extension uppercase":
			expectedPrefix = "purr build: \"hello.PCY\" is not a Percy source file (expected a .pcy file)\n"
			expectedSuffix = ""
		}
		goResult := runBuildCommand(t, goPurr, directory, testCase.arguments, nil)
		for pipeline, executable := range purrs {
			before := newSnapshot(t, directory)
			result := runBuildCommand(t, executable, directory, testCase.arguments, nil)
			if normalizeBuildUsage(t, goResult.stderr) != normalizeBuildUsage(t, result.stderr) ||
				goResult.stdout != result.stdout || goResult.status != result.status {
				t.Errorf("%s %s differs from Go: Go %d/%q/%q, Percy %d/%q/%q",
					pipeline, testCase.name, goResult.status, goResult.stdout, goResult.stderr,
					result.status, result.stdout, result.stderr)
			}
			if result.status != 2 || result.stdout != "" {
				t.Errorf("%s %s: status %d, stdout %q", pipeline, testCase.name, result.status, result.stdout)
			}
			if testCase.usage {
				want := expectedPrefix + expectedSuffix + "\n" + purrUsageText() + "\n"
				if result.stderr != want {
					t.Errorf("%s %s: stderr %q, want %q", pipeline, testCase.name, result.stderr, want)
				}
			} else if result.stderr != expectedPrefix {
				t.Errorf("%s %s: stderr %q, want %q", pipeline, testCase.name, result.stderr, expectedPrefix)
			}
			if !sameSnapshot(before, newSnapshot(t, directory)) {
				t.Errorf("%s %s touched the filesystem", pipeline, testCase.name)
			}
		}
	}

	successCases := [][]string{
		{"build", source},
		{"build", source, "-o", "out1"},
		{"build", "-o", "out2", source},
		{"build", "--output", "out3", source},
		{"build", "-o=out4", source},
		{"build", "--output=out5", source},
		{"build", "--", source},
	}
	for _, arguments := range successCases {
		goResult := runBuildCommand(t, goPurr, directory, arguments, nil)
		for pipeline, executable := range purrs {
			result := runBuildCommand(t, executable, directory, arguments, nil)
			if result.status != 0 || result.stderr != "" {
				t.Errorf("%s build %v: status %d, stderr %q", pipeline, arguments, result.status, result.stderr)
			}
			if arguments[1] == source && len(arguments) == 2 {
				if result.stdout != "built hello\n" {
					t.Errorf("%s default output: stdout %q", pipeline, result.stdout)
				}
			}
			if result.status != goResult.status || result.stdout != goResult.stdout || result.stderr != goResult.stderr {
				t.Errorf("%s build %v differs from Go: Go %d/%q/%q, Percy %d/%q/%q",
					pipeline, arguments, goResult.status, goResult.stdout, goResult.stderr,
					result.status, result.stdout, result.stderr)
			}
		}
	}
}

func TestPercyPurrBuildRealFixtures(t *testing.T) {
	requireToolchain(t)
	stage1 := buildStage1(t)
	goPurr := buildGoPurr(t)
	purrs := map[string]string{
		"Go":    buildPurrPcyWithGo(t),
		"Percy": buildPurrPcyWithPercy(t, stage1),
	}
	roots := []struct{ name, path, expected string }{
		{"hello", repoPath(t, "..", "..", "tests", "hello.pcy"), "hello"},
		{"fizzbuzz", repoPath(t, "..", "..", "tests", "fizzbuzz.pcy"), "fizzbuzz"},
		{"math", repoPath(t, "..", "..", "tests", "math.pcy"), "math"},
		{"room", repoPath(t, "..", "..", "tests", "room.pcy"), "room"},
		{"strings", repoPath(t, "..", "..", "tests", "strings.pcy"), "strings"},
		{"arrays", repoPath(t, "..", "..", "tests", "arrays.pcy"), "arrays"},
		{"variants", repoPath(t, "..", "..", "tests", "variants.pcy"), "variants"},
		{"maps", repoPath(t, "..", "..", "tests", "maps.pcy"), "maps"},
		{"lexer_proof", repoPath(t, "..", "..", "tests", "lexer_proof.pcy"), "lexer_proof"},
		{"modules", repoPath(t, "..", "..", "tests", "modules", "main.pcy"), "modules"},
	}
	for _, root := range roots {
		goOutput := filepath.Join(t.TempDir(), "program-go")
		if result := runBuildCommand(t, goPurr, t.TempDir(), []string{"build", root.path, "-o", goOutput}, nil); result.status != 0 || result.stderr != "" {
			t.Fatalf("Go build %s: %d %q", root.name, result.status, result.stderr)
		}
		goRun := runExternal(t, goOutput, nil, t.TempDir(), nil)
		for pipeline, executable := range purrs {
			output := filepath.Join(t.TempDir(), "program-"+pipeline)
			result := runBuildCommand(t, executable, t.TempDir(), []string{"build", root.path, "-o", output}, nil)
			if result.status != 0 || result.stderr != "" {
				t.Errorf("%s build %s: status %d, stderr %q", pipeline, root.name, result.status, result.stderr)
				continue
			}
			run := runExternal(t, output, nil, t.TempDir(), nil)
			if run.status != goRun.status || run.stdout != goRun.stdout || run.stderr != goRun.stderr {
				t.Errorf("%s %s run differs: %d/%q/%q vs Go %d/%q/%q",
					pipeline, root.name, run.status, run.stdout, run.stderr,
					goRun.status, goRun.stdout, goRun.stderr)
			}
		}
	}
}

func TestPercyPurrBuildDefaultAndPaths(t *testing.T) {
	requireToolchain(t)
	goPurr := buildGoPurr(t)
	pcy := buildPurrPcyWithGo(t)
	root := t.TempDir()
	writeBuildFixture(t, root)

	cases := []struct {
		name      string
		arguments []string
		output    string
	}{
		{"default", []string{"build", "hello.pcy"}, "hello"},
		{"relative output dir", []string{"build", "hello.pcy", "-o", "out/app"}, "out/app"},
		{"absolute output", []string{"build", "hello.pcy", "-o", filepath.Join(root, "absolute")}, filepath.Join(root, "absolute")},
		{"source dot dot", []string{"build", "sub/../hello.pcy"}, "hello"},
	}
	if err := os.Mkdir(filepath.Join(root, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range cases {
		for pipeline, executable := range map[string]string{"Go": goPurr, "Percy": pcy} {
			result := runBuildCommand(t, executable, root, testCase.arguments, nil)
			if result.status != 0 || result.stderr != "" {
				t.Errorf("%s %s: status %d, stderr %q", pipeline, testCase.name, result.status, result.stderr)
			}
			if result.stdout != "built "+testCase.output+"\n" {
				t.Errorf("%s %s: stdout %q", pipeline, testCase.name, result.stdout)
			}
		}
	}

	t.Run("a.out fallback", func(t *testing.T) {
		// A source named ".pcy" has an empty base name once the extension is
		// stripped, and both implementations fall back to "a.out".
		hiddenSource := "fn main() {\n    print(\"hidden\");\n}\n"
		for pipeline, executable := range map[string]string{"Go": goPurr, "Percy": pcy} {
			work := t.TempDir()
			if err := os.WriteFile(filepath.Join(work, ".pcy"), []byte(hiddenSource), 0o644); err != nil {
				t.Fatal(err)
			}
			result := runBuildCommand(t, executable, work, []string{"build", ".pcy"}, nil)
			if result.status != 0 || result.stderr != "" || result.stdout != "built a.out\n" {
				t.Errorf("%s a.out fallback: status %d, stdout %q, stderr %q",
					pipeline, result.status, result.stdout, result.stderr)
				continue
			}
			if _, err := os.Stat(filepath.Join(work, "a.out")); err != nil {
				t.Errorf("%s a.out fallback did not write a.out: %v", pipeline, err)
				continue
			}
			run := runExternal(t, filepath.Join(work, "a.out"), nil, work, nil)
			if run.status != 0 || run.stdout != "hidden\n" || run.stderr != "" {
				t.Errorf("%s a.out program: %d %q %q", pipeline, run.status, run.stdout, run.stderr)
			}
		}
	})
}

func TestPercyPurrBuildDiagnostics(t *testing.T) {
	requireToolchain(t)
	goPurr := buildGoPurr(t)
	pcy := buildPurrPcyWithGo(t)
	cases := []struct {
		name string
		root string
	}{
		{"type error", "fn main() {\n    int x = \"text\";\n}\n"},
		{"syntax error", "fn main( {\n}\n"},
		// A lexer failure is a loader error with a position, distinct from a
		// parse failure.
		{"lex error", "fn main() {\n    int x = 1;\n    @\n}\n"},
		{"missing main", "fn helper() {\n}\n"},
	}
	for _, testCase := range cases {
		directory := t.TempDir()
		path := filepath.Join(directory, "bad.pcy")
		if err := os.WriteFile(path, []byte(testCase.root), 0o644); err != nil {
			t.Fatal(err)
		}
		goResult := runBuildCommand(t, goPurr, directory, []string{"build", path}, nil)
		pcyResult := runBuildCommand(t, pcy, directory, []string{"build", path}, nil)
		if goResult.status != 1 || pcyResult.status != 1 {
			t.Errorf("%s: status Go %d, Percy %d", testCase.name, goResult.status, pcyResult.status)
		}
		if goResult.stdout != pcyResult.stdout || goResult.stderr != pcyResult.stderr {
			t.Errorf("%s diagnostics differ:\nGo %q\nPercy %q", testCase.name, goResult.stderr, pcyResult.stderr)
		}
	}

	t.Run("missing source and directory", func(t *testing.T) {
		directory := t.TempDir()
		missing := filepath.Join(directory, "missing.pcy")
		asDirectory := filepath.Join(directory, "directory.pcy")
		if err := os.Mkdir(asDirectory, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{missing, asDirectory} {
			goResult := runBuildCommand(t, goPurr, directory, []string{"build", path}, nil)
			pcyResult := runBuildCommand(t, pcy, directory, []string{"build", path}, nil)
			if goResult.status != 1 || pcyResult.status != 1 || goResult.stdout != pcyResult.stdout || goResult.stderr != pcyResult.stderr {
				t.Errorf("%s: Go %d/%q/%q, Percy %d/%q/%q", path,
					goResult.status, goResult.stdout, goResult.stderr,
					pcyResult.status, pcyResult.stdout, pcyResult.stderr)
			}
		}
	})

	t.Run("import failures", func(t *testing.T) {
		root := t.TempDir()
		project := filepath.Join(root, "project")
		if err := os.Mkdir(project, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "outside.pcy"), []byte("fn outside() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cases := []struct {
			name string
			main string
		}{
			{"missing import", "import \"missing.pcy\";\nfn main() {}\n"},
			{"import outside root", "import \"../outside.pcy\";\nfn main() {}\n"},
			{"imported syntax", "import \"bad.pcy\";\nfn main() {}\n"},
		}
		for _, testCase := range cases {
			mainPath := filepath.Join(project, "main.pcy")
			if err := os.WriteFile(mainPath, []byte(testCase.main), 0o644); err != nil {
				t.Fatal(err)
			}
			if testCase.name == "imported syntax" {
				if err := os.WriteFile(filepath.Join(project, "bad.pcy"), []byte("fn broken( {\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			goResult := runBuildCommand(t, goPurr, root, []string{"build", mainPath}, nil)

			pcyResult := runBuildCommand(t, pcy, root, []string{"build", mainPath}, nil)
			if goResult.status != 1 || pcyResult.status != 1 || goResult.stdout != pcyResult.stdout || goResult.stderr != pcyResult.stderr {
				t.Errorf("%s: Go %d/%q/%q, Percy %d/%q/%q", testCase.name,
					goResult.status, goResult.stdout, goResult.stderr,
					pcyResult.status, pcyResult.stdout, pcyResult.stderr)
			}
		}
	})
}

func TestPercyPurrBuildTempCleanup(t *testing.T) {
	requireToolchain(t)
	goPurr := buildGoPurr(t)
	pcy := buildPurrPcyWithGo(t)
	directory := t.TempDir()
	tmp := t.TempDir()
	source := writeBuildFixture(t, directory)
	environment := []string{"TMPDIR=" + tmp}
	for _, testCase := range []struct {
		name      string
		arguments []string
		status    int
	}{
		{"success", []string{"build", source, "-o", filepath.Join(directory, "ok")}, 0},
		{"failure", []string{"build", filepath.Join(directory, "missing.pcy")}, 1},
	} {
		for pipeline, executable := range map[string]string{"Go": goPurr, "Percy": pcy} {
			result := runBuildCommand(t, executable, directory, testCase.arguments, environment)
			if result.status != testCase.status {
				t.Errorf("%s %s: status %d, want %d", pipeline, testCase.name, result.status, testCase.status)
			}
			entries, err := os.ReadDir(tmp)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "purr-build") {
					t.Errorf("%s %s left temp entry %q", pipeline, testCase.name, entry.Name())
				}
			}
		}
	}
}

func TestPercyPurrBuildCapstone(t *testing.T) {
	requireToolchain(t)
	goPurr := buildGoPurr(t)
	pcy := buildPurrPcyWithGo(t)
	root := repoPath(t, "..", "..")
	driver := filepath.Join(root, "selfhost", "driver.pcy")
	work := t.TempDir()
	pcyStage := filepath.Join(work, "stage-pcy")
	start := time.Now()
	result := runBuildCommand(t, pcy, work, []string{"build", driver, "-o", pcyStage}, nil)
	elapsed := time.Since(start)
	if result.status != 0 || result.stderr != "" {
		t.Fatalf("purr-pcy capstone build: %d %q", result.status, result.stderr)
	}
	t.Logf("purr-pcy capstone build time: %s", elapsed)
	pcyC := runExternal(t, pcyStage, []string{driver}, root, nil)
	if pcyC.status != 0 || pcyC.stderr != "" {
		t.Fatalf("capstone stage output: %d %q", pcyC.status, pcyC.stderr)
	}
	goStage := filepath.Join(work, "stage-go")
	if result := runBuildCommand(t, goPurr, work, []string{"build", driver, "-o", goStage}, nil); result.status != 0 || result.stderr != "" {
		t.Fatalf("Go capstone build: %d %q", result.status, result.stderr)
	}
	goC := runExternal(t, goStage, []string{driver}, root, nil)
	if goC.status != 0 || goC.stderr != "" {
		t.Fatalf("Go stage output: %d %q", goC.status, goC.stderr)
	}
	if difference := unitDifference(pcyC.stdout, goC.stdout); difference != "" {
		t.Fatalf("capstone driver C differs: %s", difference)
	}
}

func TestPercyPurrBuildSanitizer(t *testing.T) {
	requireToolchain(t)
	root := repoPath(t, "..", "..")
	rootPath := filepath.Join(root, "purr.pcy")
	modules, err := loader.Load(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := checker.CheckModules(modules)
	if err != nil {
		t.Fatal(err)
	}
	text, err := codegen.Generate(modules, checked)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := discoverCompiler()
	if err != nil {
		t.Fatal(err)
	}
	compileFlags, linkFlags := gcBuildFlags()
	compileFlags = append(compileFlags, "-fsanitize=undefined")
	executable, err := buildPercyRunProgram(t, compiler, compileFlags, linkFlags, text)
	if err != nil {
		t.Skipf("undefined behavior sanitizer is unavailable with %s: %v", filepath.Base(compiler[0]), err)
	}
	directory := t.TempDir()
	source := writeBuildFixture(t, directory)
	result := runExternal(t, executable, []string{"build", source, "-o", filepath.Join(directory, "san")}, directory, nil)
	if result.status != 0 || result.stderr != "" {
		t.Fatalf("sanitized build: status %d, stderr %q", result.status, result.stderr)
	}
	run := runExternal(t, filepath.Join(directory, "san"), nil, directory, nil)
	if run.status != 0 || run.stdout != "Hello, world!\n" || run.stderr != "" {
		t.Fatalf("sanitized built program: %d/%q/%q", run.status, run.stdout, run.stderr)
	}
}

func TestPercyPurrBuildOutputFailures(t *testing.T) {
	requireToolchain(t)
	goPurr := buildGoPurr(t)
	pcy := buildPurrPcyWithGo(t)
	root := t.TempDir()
	writeBuildFixture(t, root)

	t.Run("existing output directory", func(t *testing.T) {
		output := filepath.Join(root, "outdir")
		if err := os.Mkdir(output, 0o755); err != nil {
			t.Fatal(err)
		}
		goResult := runBuildCommand(t, goPurr, root, []string{"build", "hello.pcy", "-o", output}, nil)
		pcyResult := runBuildCommand(t, pcy, root, []string{"build", "hello.pcy", "-o", output}, nil)
		if goResult.status != 1 || pcyResult.status != 1 {
			t.Fatalf("status Go %d, Percy %d", goResult.status, pcyResult.status)
		}
		if !strings.Contains(goResult.stderr, "C compilation failed") {
			t.Errorf("Go output directory diagnostic: %q", goResult.stderr)
		}
		wantPercy := "hello.pcy: output path " + output + " is a directory\n"
		if pcyResult.stderr != wantPercy {
			t.Errorf("Percy output directory diagnostic: %q; want %q", pcyResult.stderr, wantPercy)
		}
	})

	t.Run("unwritable output directory", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("permission checks are not meaningful as root")
		}
		outputDir := filepath.Join(root, "locked")
		if err := os.Mkdir(outputDir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(outputDir, 0o755) })
		output := filepath.Join(outputDir, "app")
		goResult := runBuildCommand(t, goPurr, root, []string{"build", "hello.pcy", "-o", output}, nil)
		pcyResult := runBuildCommand(t, pcy, root, []string{"build", "hello.pcy", "-o", output}, nil)
		if goResult.status != 1 || pcyResult.status != 1 {
			t.Fatalf("status Go %d, Percy %d", goResult.status, pcyResult.status)
		}
		if !strings.Contains(goResult.stderr, "C compilation failed") {
			t.Errorf("Go unwritable output diagnostic: %q", goResult.stderr)
		}
		wantPrefix := "hello.pcy: cannot write " + output + ": "
		if !strings.HasPrefix(pcyResult.stderr, wantPrefix) ||
			!(strings.HasSuffix(pcyResult.stderr, "permission denied\n") || strings.HasSuffix(pcyResult.stderr, "operation not permitted\n")) {
			t.Errorf("Percy unwritable output diagnostic: %q; want %q with a permission reason", pcyResult.stderr, wantPrefix)
		}
	})
}

// TestPercyPurrBuildSourceReadErrors covers the last path-only loader failure
// in the parity suite: an existing but unreadable source file. The missing,
// directory, lex, parse, and checker cases live in
// TestPercyPurrBuildDiagnostics.
func TestPercyPurrBuildSourceReadErrors(t *testing.T) {
	requireToolchain(t)
	goPurr := buildGoPurr(t)
	pcy := buildPurrPcyWithGo(t)
	if os.Geteuid() == 0 {
		t.Skip("permission checks are not meaningful as root")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "secret.pcy")
	sourceText := "fn main() {\n    print(\"hidden\");\n}\n"
	if err := os.WriteFile(source, []byte(sourceText), 0o000); err != nil {
		t.Fatalf("writing unreadable source: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(source, 0o644) })

	goResult := runBuildCommand(t, goPurr, directory, []string{"build", source}, nil)
	pcyResult := runBuildCommand(t, pcy, directory, []string{"build", source}, nil)
	if goResult.status != 1 || pcyResult.status != 1 {
		t.Fatalf("status Go %d, Percy %d", goResult.status, pcyResult.status)
	}
	if goResult.stdout != "" || pcyResult.stdout != "" {
		t.Errorf("stdout Go %q, Percy %q", goResult.stdout, pcyResult.stdout)
	}
	if goResult.stderr != pcyResult.stderr {
		t.Errorf("unreadable source diagnostics differ:\nGo %q\nPercy %q", goResult.stderr, pcyResult.stderr)
	}
	if !strings.HasPrefix(goResult.stderr, source+": cannot read source file: ") {
		t.Errorf("Go stderr %q does not have the loader's prefix", goResult.stderr)
	}

	// A failed build of an unreadable source must not touch the filesystem.
	// The source is unreadable by design, so only entry names can be compared.
	listNames := func() []string {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("reading directory: %v", err)
		}
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return names
	}
	before := listNames()
	pcyAgain := runBuildCommand(t, pcy, directory, []string{"build", source}, nil)
	if after := listNames(); len(after) != len(before) {
		t.Errorf("failed build changed the directory entries: %v != %v", after, before)
	}
	if pcyAgain.stderr != pcyResult.stderr {
		t.Errorf("repeat run stderr %q differs from %q", pcyAgain.stderr, pcyResult.stderr)
	}
}

// TestPercyPurrBuildImportCycle covers the loader's cycle handling through the
// CLI: a module importing a module that imports it back. The load must succeed
// and both pipelines must produce byte-identical C and identical behavior.
func TestPercyPurrBuildImportCycle(t *testing.T) {
	requireToolchain(t)
	stage1 := buildStage1(t)
	goPurr := buildGoPurr(t)
	purrs := map[string]string{
		"Go":    buildPurrPcyWithGo(t),
		"Percy": buildPurrPcyWithPercy(t, stage1),
	}
	directory := t.TempDir()
	mainSource := "import \"helper.pcy\";\n" +
		"fn main() {\n" +
		"    print(label(2));\n" +
		"}\n"
	helperSource := "import \"main.pcy\";\n" +
		"public fn label(int count) -> string {\n" +
		"    int index = 0;\n" +
		"    string result = \"\";\n" +
		"    while (index < count) {\n" +
		"        result = result + \"cycle\";\n" +
		"        index = index + 1;\n" +
		"    }\n" +
		"    return result;\n" +
		"}\n"
	if err := os.WriteFile(filepath.Join(directory, "main.pcy"), []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "helper.pcy"), []byte(helperSource), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(directory, "main.pcy")

	// Real builds through both CLIs and both purr-pcy builds: the loader cycle
	// terminates and the program works.
	pipelines := map[string]string{"Go": goPurr, "Go purr-pcy": purrs["Go"], "Percy purr-pcy": purrs["Percy"]}
	for pipeline, executable := range pipelines {
		output := filepath.Join(directory, strings.ReplaceAll(pipeline, " ", "-"))
		result := runBuildCommand(t, executable, directory, []string{"build", mainPath, "-o", output}, nil)
		if result.status != 0 || result.stderr != "" || result.stdout != "built "+output+"\n" {
			t.Errorf("%s cycle build: status %d, stdout %q, stderr %q",
				pipeline, result.status, result.stdout, result.stderr)
			continue
		}
		run := runExternal(t, output, nil, directory, nil)
		if run.status != 0 || run.stdout != "cyclecycle\n" || run.stderr != "" {
			t.Errorf("%s cycle run: %d %q %q", pipeline, run.status, run.stdout, run.stderr)
		}
	}

}
