package checker

import (
	"strings"

	"percy/parser"
)

// runtimeIntrinsicFunctions returns the functions that resolve only while
// checking the synthetic <runtime> module. They are not in the ordinary
// built-in table, so user modules cannot resolve them. A function declared in
// the runtime module with the same name wins because lookupFunction runs first.
func runtimeIntrinsicFunctions() map[string]*Function {
	intType := IntType
	byteType := ByteType
	voidType := VoidType
	return map[string]*Function{
		"raw_load8": {
			Name:           "raw_load8",
			ParameterTypes: []*Type{intType},
			ReturnType:     byteType,
			Builtin:        BuiltinRawLoad8,
		},
		"raw_store8": {
			Name:           "raw_store8",
			ParameterTypes: []*Type{intType, byteType},
			ReturnType:     voidType,
			Builtin:        BuiltinRawStore8,
		},
		"raw_load64": {
			Name:           "raw_load64",
			ParameterTypes: []*Type{intType},
			ReturnType:     intType,
			Builtin:        BuiltinRawLoad64,
		},
		"raw_store64": {
			Name:           "raw_store64",
			ParameterTypes: []*Type{intType, intType},
			ReturnType:     voidType,
			Builtin:        BuiltinRawStore64,
		},
		"raw_copy": {
			Name:           "raw_copy",
			ParameterTypes: []*Type{intType, intType, intType},
			ReturnType:     voidType,
			Builtin:        BuiltinRawCopy,
		},
		"raw_zero": {
			Name:           "raw_zero",
			ParameterTypes: []*Type{intType, intType},
			ReturnType:     voidType,
			Builtin:        BuiltinRawZero,
		},
		"address_of": {
			Name:           "address_of",
			ParameterTypes: []*Type{intType},
			ReturnType:     intType,
			Builtin:        BuiltinAddressOf,
		},
		"syscall": {
			Name:           "syscall",
			ParameterTypes: []*Type{intType, intType, intType, intType, intType, intType, intType},
			ReturnType:     intType,
			Builtin:        BuiltinSyscall,
		},
	}
}

// isReservedRuntimeModulePath reports whether a module may use intrinsics:
// the synthetic <runtime> module and the compiler's reserved runtime/pcy
// helper modules. The paths are reserved so a user module cannot quietly gain
// the raw surface by choosing a different file name.
func isReservedRuntimeModulePath(path string) bool {
	if path == runtimeModulePath {
		return true
	}
	return strings.HasSuffix(path, "/runtime/pcy/memory.pcy") ||
		strings.HasSuffix(path, "/runtime/pcy/io.pcy")
}

// lookupRuntimeIntrinsic resolves an intrinsic only in a reserved runtime
// module.
func (c *checker) lookupRuntimeIntrinsic(name string) (*Function, bool) {
	if c.current == nil || !isReservedRuntimeModulePath(c.current.path) {
		return nil, false
	}
	function, ok := runtimeIntrinsicFunctions()[name]
	return function, ok
}

// checkAddressOfCall checks the overloaded address_of intrinsic. Its argument
// must be a byte array or a string; the result is always int.
func (c *checker) checkAddressOfCall(expression *parser.CallExpr, function *Function, scope *Scope) (*Type, error) {
	if len(expression.Arguments) != 1 {
		return nil, c.errorAt(BadCall, expression.OpenParen, "function %q expects 1 argument(s), got %d", function.Name, len(expression.Arguments))
	}
	argument := expression.Arguments[0]
	argumentType, err := c.checkExpr(argument, scope)
	if err != nil {
		return nil, err
	}
	if argumentType == StringType || (argumentType.Kind == KindArray && argumentType.Element == ByteType) {
		return IntType, nil
	}
	return nil, c.errorAt(TypeMismatch, argument.Pos(), "argument 1 of %q expected byte[] or string, found %s", function.Name, argumentType)
}
