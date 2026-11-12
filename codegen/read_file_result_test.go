package codegen

import (
	"strings"
	"testing"

	"percy/checker"
	"percy/lexer"
	"percy/parser"
)

func parseReadFileModule(t *testing.T, path string, id int, source string) *parser.Module {
	t.Helper()

	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lexing %s: %v", path, err)
	}
	program, err := parser.Parse(tokens)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return &parser.Module{Path: path, ID: id, Program: program}
}

func TestTryReadFileCIsExact(t *testing.T) {
	const source = `fn main() {
    match (try_read_file("x")) {
        Ok(text) { print(text); }
        Failed(message) { print(message); }
    }
}
`
	generated := generateSource(t, source)

	expectedForward := `typedef struct pcy_s_1_ReadFileResult pcy_s_1_ReadFileResult;

enum {
    pcy_uc_1_ReadFileResult_Ok = 0,
    pcy_uc_1_ReadFileResult_Failed = 1,
};
`
	expectedStruct := `struct pcy_s_1_ReadFileResult {
    int64_t pcy_tag;
    const pcy_string * pcy_v_Ok_text;
    const pcy_string * pcy_v_Failed_message;
};
`
	expectedOkConstructor := `pcy_s_1_ReadFileResult *pcy_new_1_ReadFileResult_Ok(const pcy_string * pcy_arg_text) {
    pcy_s_1_ReadFileResult *pcy_object = (pcy_s_1_ReadFileResult *)pcy_rt_alloc(sizeof(pcy_s_1_ReadFileResult));
    pcy_object->pcy_tag = pcy_uc_1_ReadFileResult_Ok;
    pcy_object->pcy_v_Ok_text = pcy_arg_text;
    return pcy_object;
}
`
	expectedFailedConstructor := `pcy_s_1_ReadFileResult *pcy_new_1_ReadFileResult_Failed(const pcy_string * pcy_arg_message) {
    pcy_s_1_ReadFileResult *pcy_object = (pcy_s_1_ReadFileResult *)pcy_rt_alloc(sizeof(pcy_s_1_ReadFileResult));
    pcy_object->pcy_tag = pcy_uc_1_ReadFileResult_Failed;
    pcy_object->pcy_v_Failed_message = pcy_arg_message;
    return pcy_object;
}
`
	expectedHelper := `pcy_s_1_ReadFileResult *pcy_prelude_1_try_read_file(const pcy_string *pcy_path) {
    const pcy_string *pcy_text = NULL;
    const pcy_string *pcy_message = NULL;
    if (pcy_rt_try_read_file(pcy_path, &pcy_text, &pcy_message)) {
        return pcy_new_1_ReadFileResult_Ok(pcy_text);
    }
    return pcy_new_1_ReadFileResult_Failed(pcy_message);
}
`
	assertContains(t, generated, expectedForward)
	assertContains(t, generated, expectedStruct)
	assertContains(t, generated, expectedOkConstructor)
	assertContains(t, generated, expectedFailedConstructor)
	assertContains(t, generated, expectedHelper)
	if !strings.Contains(generated, "pcy_prelude_1_try_read_file(") {
		t.Errorf("try_read_file call was not lowered to the prelude helper:\n%s", generated)
	}
}

func TestTryReadFileDoesNotChangePlainProgram(t *testing.T) {
	generated := generateSource(t, "fn main() {\n}\n")
	if strings.Contains(generated, "ReadFileResult") || strings.Contains(generated, "pcy_prelude_") {
		t.Errorf("plain program gained prelude output:\n%s", generated)
	}
}

func TestTryReadFilePreludeUsesCheckedModules(t *testing.T) {
	root := parseReadFileModule(t, "input.txt", 0, `import "input2.txt";
fn main() {
    match (try_read_file("x")) {
        Ok(text) { print(text); }
        Failed(message) { print(message); }
    }
}
`)
	imported := parseReadFileModule(t, "input2.txt", 1, "public struct S {\n    int value;\n}\n")
	modules := []*parser.Module{root, imported}
	checked, err := checker.CheckModules(modules)
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	// Pass only the pre-prelude set: Generate must use checked.Modules.
	generated, err := Generate(modules, checked)
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	if !strings.Contains(generated, "pcy_s_1_S") {
		t.Errorf("imported type was renumbered:\n%s", generated)
	}
	if !strings.Contains(generated, "pcy_s_2_ReadFileResult") ||
		!strings.Contains(generated, "pcy_prelude_2_try_read_file") {
		t.Errorf("prelude was not appended at module id 2:\n%s", generated)
	}
}
