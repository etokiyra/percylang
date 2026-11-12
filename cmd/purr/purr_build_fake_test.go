package main

// Fake-compiler helpers retained for the surviving build tests.

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func writeFakeCompiler(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "fake-cc")
	script := `#!/bin/sh
log=${FAKE_LOG:?}
{
  printf 'cwd=%s\n' "$PWD"
  printf 'argc=%s\n' "$#"
  index=0
  for argument in "$@"; do
    printf 'arg%s=%s\n' "$index" "$argument"
    index=$((index + 1))
  done
} > "$log.args"
cp program.c "$log.program.c"
cp percy_runtime.c "$log.runtime.c"
cp percy_runtime.h "$log.runtime.h"
mode=${FAKE_MODE:-success}
case "$mode" in
  success)
    exec "$REAL_CC" "$@"
    ;;
  warnings)
    printf 'fake compiler warning\n' >&2
    exec "$REAL_CC" "$@"
    ;;
  fail)
    printf 'fake compiler error\n' >&2
    exit 1
    ;;
  both)
    printf 'fake stdout\n'
    printf 'fake stderr\n' >&2
    exit 1
    ;;
  args-fail)
    printf 'fake compiler diagnostic\n' >&2
    index=0
    for argument in "$@"; do
      printf 'arg%s=%s\n' "$index" "$argument" >&2
      index=$((index + 1))
    done
    exit 1
    ;;
  no-output)
    exit 0
    ;;
  directory)
    mkdir -p "$FAKE_OUTPUT"
    exit 0
    ;;
  signal)
    kill -TERM $$
    ;;
  extra-success)
    touch extra
    exec "$REAL_CC" "$@"
    ;;
  extra-fail)
    touch extra
    printf 'fake compiler error\n' >&2
    exit 1
    ;;
  fake-output)
    : > "$FAKE_OUTPUT"
    exit 0
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake compiler: %v", err)
	}
	return path
}

func fakeBuildEnvironment(t *testing.T, fake, log, mode, output, tmp string) []string {
	environment := []string{
		"CC=" + fake,
		"REAL_CC=" + realCompilerPath(t),
		"FAKE_LOG=" + log,
		"FAKE_MODE=" + mode,
		"TMPDIR=" + tmp,
	}
	if output != "" {
		environment = append(environment, "FAKE_OUTPUT="+output)
	}
	return environment
}

func realCompilerPath(t *testing.T) string {
	t.Helper()
	compiler, err := discoverCompiler()
	if err != nil {
		t.Fatalf("discovering real compiler: %v", err)
	}
	return compiler[0]
}

var buildDirectoryPattern = regexp.MustCompile(`purr-build[.-][A-Za-z0-9]+`)

func normalizeBuildLog(text string) string {
	return buildDirectoryPattern.ReplaceAllString(text, "purr-build.X")
}

func readFakeLog(t *testing.T, prefix, suffix string) string {
	t.Helper()
	data, err := os.ReadFile(prefix + suffix)
	if err != nil {
		t.Fatalf("reading fake log %s%s: %v", prefix, suffix, err)
	}
	return string(data)
}
