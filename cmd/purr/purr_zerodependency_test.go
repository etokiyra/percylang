package main

// TestPercyPurrZeroDependency is the zero-dependency verification rung from
// proposals/zero-dependency-toolchain.md Section 8: the shipped Percy purr is
// the only executable on PATH, with no Go, C compiler, assembler, linker,
// pkg-config, or libgc reachable. It builds and runs every fixture and
// tests/modules, its outputs are static, and it builds itself with a native
// stage fixpoint.
//
// Building the purr under test still uses the Go bootstrap (the frozen stage-1
// tool); everything the test *runs* uses the copied purr and the built
// programs only.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPercyPurrZeroDependency(t *testing.T) {
	requireToolchain(t)
	bootstrapBuilt := buildPurrPcyWithGo(t)

	tools := t.TempDir()
	installed := filepath.Join(tools, "purr")
	contents, err := os.ReadFile(bootstrapBuilt)
	if err != nil {
		t.Fatalf("reading the Percy purr: %v", err)
	}
	if err := os.WriteFile(installed, contents, 0o755); err != nil {
		t.Fatalf("installing the Percy purr: %v", err)
	}

	temp := t.TempDir()
	environment := []string{
		"PATH=" + tools,
		"CC=/nonexistent",
		"TMPDIR=" + temp,
		"HOME=" + temp,
	}
	work := t.TempDir()
	runMinimal := func(executable string, arguments []string, directory string) programResult {
		command := exec.Command(executable, arguments...)
		command.Dir = directory
		command.Env = environment
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		status := 0
		if err := command.Run(); err != nil {
			var exit *exec.ExitError
			if !asExitError(err, &exit) {
				t.Fatalf("running %s: %v", executable, err)
			}
			status = exit.ExitCode()
		}
		return programResult{stdout: stdout.String(), stderr: stderr.String(), status: status}
	}

	assertStatic := func(path string) {
		t.Helper()
		if _, err := exec.LookPath("readelf"); err != nil {
			t.Logf("readelf is unavailable; skipping the static check for %s", path)
			return
		}
		readelf := runExternal(t, "readelf", []string{"-d", path}, work, nil)
		if readelf.status != 0 {
			t.Fatalf("readelf -d %s: status %d, stderr %q", path, readelf.status, readelf.stderr)
		}
		lower := strings.ToLower(readelf.stdout)
		if strings.Contains(lower, "needed") || strings.Contains(lower, "interpreter") {
			t.Fatalf("%s is not static:\n%s", path, readelf.stdout)
		}
	}

	buildAndRun := func(source, expectedPath, name string) {
		t.Helper()
		expected, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Fatalf("reading expected output for %s: %v", name, err)
		}
		output := filepath.Join(work, name)
		built := runMinimal(installed, []string{"build", source, "-o", output}, work)
		if built.status != 0 || built.stderr != "" || built.stdout != "built "+output+"\n" {
			t.Fatalf("%s native build: status %d, stdout %q, stderr %q",
				name, built.status, built.stdout, built.stderr)
		}
		assertStatic(output)
		run := runMinimal(output, nil, work)
		if run.status != 0 || run.stderr != "" || run.stdout != string(expected) {
			t.Fatalf("%s native run: status %d, stdout %q, stderr %q",
				name, run.status, run.stdout, run.stderr)
		}
	}

	fixtures, err := filepath.Glob(repoPath(t, "..", "..", "tests", "*.pcy"))
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("globbing fixtures: %v (%d)", err, len(fixtures))
	}
	builtFixtures := 0
	for _, source := range fixtures {
		name := strings.TrimSuffix(filepath.Base(source), ".pcy")
		expectedPath := filepath.Join(filepath.Dir(source), name+".expected")
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			continue
		}
		buildAndRun(source, expectedPath, name)
		builtFixtures++
	}
	if builtFixtures < 9 {
		t.Fatalf("built %d fixtures, want at least 9", builtFixtures)
	}

	buildAndRun(
		repoPath(t, "..", "..", "tests", "modules", "main.pcy"),
		repoPath(t, "..", "..", "tests", "modules", "main.expected"),
		"modules",
	)

	// The production native fixpoint: the shipped purr builds itself, and the
	// next stage reproduces the same ELF.
	source := repoPath(t, "..", "..", "purr.pcy")
	stageB := filepath.Join(work, "purr-stage-b")
	builtB := runMinimal(installed, []string{"build", source, "-o", stageB}, work)
	if builtB.status != 0 || builtB.stderr != "" || builtB.stdout != "built "+stageB+"\n" {
		t.Fatalf("stage B build: status %d, stdout %q, stderr %q",
			builtB.status, builtB.stdout, builtB.stderr)
	}
	stageC := filepath.Join(work, "purr-stage-c")
	builtC := runMinimal(stageB, []string{"build", source, "-o", stageC}, work)
	if builtC.status != 0 || builtC.stderr != "" || builtC.stdout != "built "+stageC+"\n" {
		t.Fatalf("stage C build: status %d, stdout %q, stderr %q",
			builtC.status, builtC.stdout, builtC.stderr)
	}
	first, err := os.ReadFile(stageB)
	if err != nil {
		t.Fatalf("reading stage B: %v", err)
	}
	second, err := os.ReadFile(stageC)
	if err != nil {
		t.Fatalf("reading stage C: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("native purr stage B and stage C differ in %d and %d bytes", len(first), len(second))
	}
	assertStatic(stageB)

	help := runMinimal(stageB, []string{"help"}, work)
	if help.status != 0 || help.stderr != "" || !strings.HasPrefix(help.stdout, "Percy compiler\n") {
		t.Fatalf("stage B help: status %d, stdout %q, stderr %q", help.status, help.stdout, help.stderr)
	}
}
