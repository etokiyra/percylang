package codegen

import (
	"fmt"
	"strconv"

	"percy/checker"
	"percy/parser"
)

// Generated C identifiers use reserved prefixes so that arbitrary Percy
// identifiers can never collide with C keywords, with runtime and entry-point
// names, or with each other. Percy identifiers are ASCII letters, digits, and
// underscores, so prefixing always yields a valid C identifier.
const (
	functionPrefix     = "pcy_fn_"
	variablePrefix     = "pcy_var_"
	structPrefix       = "pcy_s_"
	fieldPrefix        = "pcy_f_"
	constructorPrefix  = "pcy_new_"
	argumentPrefix     = "pcy_arg_"
	enumPrefix         = "pcy_e_"
	enumCasePrefix     = "pcy_ec_"
	unionTagPrefix     = "pcy_uc_"
	variantFieldPrefix = "pcy_v_"
	matchPrefix        = "pcy_match_"
	literalPrefix      = "pcy_literal_"

	// objectName is the local used inside generated struct constructors.
	objectName = "pcy_object"

	// tagFieldName is the C member holding a union value's active variant tag.
	tagFieldName = "pcy_tag"
)

// modulePrefix returns the C name prefix for a module. The root module (ID 0)
// has no prefix, so single-module programs generate the same names as before.
// Imported modules use "<id>_"; because a Percy identifier cannot begin with a
// digit, a mangled imported name can never collide with a root-module name, and
// distinct IDs keep imported modules distinct.
func modulePrefix(module int) string {
	if module <= 0 {
		return ""
	}
	return strconv.Itoa(module) + "_"
}

// functionName maps a Percy function name in a module to its C identifier.
func functionName(name string, module int) string {
	return functionPrefix + modulePrefix(module) + name
}

// variableName maps a Percy variable or parameter name to its C identifier.
// Local names are not module-scoped, so they are not mangled.
func variableName(name string) string { return variablePrefix + name }

// structName maps a Percy struct name in a module to its C struct type name.
func structName(name string, module int) string {
	return structPrefix + modulePrefix(module) + name
}

// fieldName maps a Percy struct field name to its C member name.
func fieldName(name string) string { return fieldPrefix + name }

// constructorName maps a Percy struct name in a module to the name of the
// function that allocates and initializes one instance.
func constructorName(name string, module int) string {
	return constructorPrefix + modulePrefix(module) + name
}

// argumentName maps a struct field name to the parameter name used in the
// generated constructor.
func argumentName(name string) string { return argumentPrefix + name }

// enumName maps a Percy enum type to its C typedef name.
func enumName(name string, module int) string {
	return enumPrefix + modulePrefix(module) + name
}

// enumCaseName maps an enum case to its C constant name.
func enumCaseName(enumType *checker.Type, caseName string) string {
	return enumCasePrefix + modulePrefix(enumType.Module) + enumType.Name + "_" + caseName
}

// unionTagName maps a union variant to its C tag constant name.
func unionTagName(unionType *checker.Type, variantName string) string {
	return unionTagPrefix + modulePrefix(unionType.Module) + unionType.Name + "_" + variantName
}

// variantFieldName maps a union variant's field to its C member name in the flat
// union layout. The variant name is part of the member so that variants may
// reuse field names.
func variantFieldName(variantName, fieldName string) string {
	return variantFieldPrefix + variantName + "_" + fieldName
}

// variantConstructorName maps a union variant to the function that allocates and
// initializes one instance carrying that variant.
func variantConstructorName(unionType *checker.Type, variantName string) string {
	return constructorPrefix + modulePrefix(unionType.Module) + unionType.Name + "_" + variantName
}

// preludeTryReadFileHelperName is the generated helper that builds the
// predeclared ReadFileResult from the runtime's non-fatal read primitive.
func preludeTryReadFileHelperName(module int) string {
	return "pcy_prelude_" + strconv.Itoa(module) + "_try_read_file"
}

// preludeHelperName is the generated helper for one used file or directory
// built-in in the prelude module. The read helper keeps its original name.
func preludeHelperName(module int, builtin string) string {
	return "pcy_prelude_" + strconv.Itoa(module) + "_" + builtin
}

// cType maps a resolved Percy type to its C representation:
//
//	int    -> int64_t             (signed 64-bit)
//	float  -> double              (IEEE 754 binary64)
//	bool   -> bool                (stdbool.h)
//	string -> const pcy_string *  (immutable, length-carrying byte string)
//	void   -> void                (function return position only)
//	struct -> struct pointer      (GC-managed reference)
//	array  -> pcy_array *         (GC-managed dynamic array)
//
// A Percy string is a pointer to the runtime's pcy_string, which stores the
// byte length next to the bytes, so length and indexing are constant-time and
// the bytes are still NUL-terminated. The pointer is const-qualified because
// strings are immutable; a static literal is a `static const pcy_string` and a
// constructed string is a GC-managed pcy_string that the runtime fills before
// returning it.
func cType(percyType *checker.Type) (string, error) {
	switch {
	case percyType == nil:
		return "", fmt.Errorf("missing type")
	case percyType == checker.IntType:
		return "int64_t", nil
	case percyType == checker.FloatType:
		return "double", nil
	case percyType == checker.BoolType:
		return "bool", nil
	case percyType == checker.StringType:
		return "const pcy_string *", nil
	case percyType == checker.ByteType:
		return "uint8_t", nil
	case percyType == checker.VoidType:
		return "void", nil
	case percyType.Kind == checker.KindArray:
		return "pcy_array *", nil
	case percyType.Kind == checker.KindMap:
		return "pcy_map *", nil
	case percyType.Kind == checker.KindEnum:
		return enumName(percyType.Name, percyType.Module), nil
	case percyType.Kind == checker.KindUnion, percyType.Kind == checker.KindStruct:
		return structName(percyType.Name, percyType.Module) + " *", nil
	default:
		return "", fmt.Errorf("unsupported type %q", percyType)
	}
}

// declaredType returns the resolved type of a declared type reference, as
// recorded by the checker. Code generation never re-resolves names.
func (g *generator) declaredType(reference *parser.TypeRef) (*checker.Type, error) {
	if reference == nil {
		return nil, &Error{Line: 1, Column: 1, Message: "missing type reference"}
	}
	resolved, ok := g.checked.TypeRefs[reference]
	if !ok || resolved == nil {
		return nil, &Error{Line: reference.At.Line, Column: reference.At.Column, Message: fmt.Sprintf("unresolved type %q", reference.Name)}
	}
	return resolved, nil
}

// expressionType returns the type the checker recorded for an expression.
func (g *generator) expressionType(expression parser.Expr) (*checker.Type, error) {
	if expression == nil {
		return nil, &Error{Line: 1, Column: 1, Message: "missing expression"}
	}
	resolved, ok := g.checked.ExpressionTypes[expression]
	if !ok || resolved == nil {
		return nil, &Error{Line: expression.Pos().Line, Column: expression.Pos().Column, Message: "expression has no recorded type"}
	}
	return resolved, nil
}
