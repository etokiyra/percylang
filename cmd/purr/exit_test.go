package main

// Runtime tests for the exit() built-in. The generated C is compared between the
// Go and Percy pipelines, then each pipeline's executable is run and stdout,
// stderr, and the process exit status are compared exactly.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exitCase is one program and the observable result of running it.
type exitCase struct {
	description string
	source      string
	stdout      string
	stderr      string
	status      int
}

func exitCases() []exitCase {
	return []exitCase{
		{
			description: "exit zero after print",
			source: `fn main() {
    print("before zero");
    exit(0);
    print("after zero");
}
`,
			stdout: "before zero\n",
			status: 0,
		},
		{
			description: "exit three after print and eprint",
			source: `fn main() {
    print("out before three");
    eprint("err before three");
    exit(3);
}
`,
			stdout: "out before three\n",
			stderr: "err before three\n",
			status: 3,
		},
		{
			description: "exit 256 is observed as zero",
			source: `fn main() {
    print("wrap");
    exit(256);
}
`,
			stdout: "wrap\n",
			status: 0,
		},
		{
			description: "exit minus one is observed as 255",
			source: `fn main() {
    exit(-1);
}
`,
			status: 255,
		},
		{
			description: "exit from a nested function",
			source: `fn stop() {
    print("nested out");
    eprint("nested err");
    exit(4);
}
fn main() {
    stop();
}
`,
			stdout: "nested out\n",
			stderr: "nested err\n",
			status: 4,
		},
		{
			description: "exit from inside a loop",
			source: `fn main() {
    int index = 0;
    while (index < 3) {
        print("step", index);
        if (index == 1) {
            exit(5);
        }
        index++;
    }
}
`,
			stdout: "step 0\nstep 1\n",
			status: 5,
		},
	}
}

// TestExitRuntime checks stdout, stderr, and the exit status through both
// pipelines for every scenario.
func TestExitRuntime(t *testing.T) {
	requireToolchain(t)

	percyExecutable := buildPercyUnit(t)
	compiler, err := discoverCompiler()
	if err != nil {
		t.Fatalf("discovering the C compiler: %v", err)
	}
	compileFlags, linkFlags := gcBuildFlags()

	for _, testCase := range exitCases() {
		sourceDirectory := t.TempDir()
		sourcePath := filepath.Join(sourceDirectory, "main.pcy")
		if err := os.WriteFile(sourcePath, []byte(testCase.source), 0o644); err != nil {
			t.Fatalf("%s: writing main.pcy: %v", testCase.description, err)
		}

		goExecutable := filepath.Join(sourceDirectory, "go-program")
		var diagnostics strings.Builder
		if err := buildExecutable(sourcePath, goExecutable, &diagnostics); err != nil {
			t.Fatalf("%s: building the Go-pipeline program: %v\n%s", testCase.description, err, diagnostics.String())
		}

		percyProbeDirectory := t.TempDir()
		percyText, err := runPercyUnit(percyExecutable, percyProbeDirectory, testCase.source, "")
		if err != nil {
			t.Fatalf("%s: %v", testCase.description, err)
		}
		_, _, generated, ok := prepareUnit(t, testCase.description, testCase.source, "")
		if !ok {
			t.Fatalf("%s: the reference rejected the program", testCase.description)
		}
		if difference := unitDifference(percyText, generated); difference != "" {
			t.Fatalf("%s: the translation units differ: %s", testCase.description, difference)
		}

		goResult := runProgramWithArgs(t, goExecutable, sourceDirectory, nil)
		if goResult.stdout != testCase.stdout || goResult.stderr != testCase.stderr || goResult.status != testCase.status {
			t.Errorf("%s: Go pipeline printed stdout %q (want %q), stderr %q (want %q), exited %d (want %d)",
				testCase.description, goResult.stdout, testCase.stdout, goResult.stderr, testCase.stderr, goResult.status, testCase.status)
		}

		percyDirectory := t.TempDir()
		percyResult, err := buildAndRunProgramWithArgs(t, compiler, compileFlags, linkFlags, percyDirectory, percyText, 0, nil)
		if err != nil {
			t.Fatalf("%s: Percy pipeline: %v", testCase.description, err)
		}
		if percyResult.stdout != testCase.stdout || percyResult.stderr != testCase.stderr || percyResult.status != testCase.status {
			t.Errorf("%s: Percy pipeline printed stdout %q (want %q), stderr %q (want %q), exited %d (want %d)",
				testCase.description, percyResult.stdout, testCase.stdout, percyResult.stderr, testCase.stderr, percyResult.status, testCase.status)
		}
	}
	t.Logf("exit: %d scenarios matched stdout, stderr, and status under %s", len(exitCases()), filepath.Base(compiler[0]))
}
