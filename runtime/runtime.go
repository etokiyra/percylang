package runtime

import _ "embed"

// The runtime C sources are embedded so the compiler can write them next to
// generated code without depending on where the Percy repository lives.

// The C sources live in the csrc subdirectory so the Go build does not treat
// them as package C sources.

//go:embed csrc/percy_runtime.h
var headerSource string

//go:embed csrc/percy_runtime.c
var sourceSource string

// File names for the runtime sources. The generated program includes
// HeaderFileName, and both files are compiled together with the generated code.
const (
	HeaderFileName = "percy_runtime.h"
	SourceFileName = "percy_runtime.c"
)

// Runtime ABI names. Generated code calls these functions; keeping the names in
// one place documents the interface between the code generator and the runtime.
const (
	// InitFunction initializes the garbage collector.
	InitFunction = "pcy_rt_init"

	// AllocFunction allocates a zeroed, GC-managed object.
	AllocFunction = "pcy_rt_alloc"

	// I64AddFunction, I64SubFunction, I64MulFunction, I64NegFunction,
	// I64DivFunction, and I64ModFunction implement defined two's-complement
	// arithmetic. The assignment and increment helpers take the address of the
	// target exactly once.
	I64AddFunction       = "pcy_rt_i64_add"
	I64SubFunction       = "pcy_rt_i64_sub"
	I64MulFunction       = "pcy_rt_i64_mul"
	I64NegFunction       = "pcy_rt_i64_neg"
	I64DivFunction       = "pcy_rt_i64_div"
	I64ModFunction       = "pcy_rt_i64_mod"
	I64ShlFunction       = "pcy_rt_i64_shl"
	I64ShrFunction       = "pcy_rt_i64_shr"
	I64AddAssignFunction = "pcy_rt_i64_add_assign"
	I64SubAssignFunction = "pcy_rt_i64_sub_assign"
	I64PreIncFunction    = "pcy_rt_i64_pre_inc"
	I64PostIncFunction   = "pcy_rt_i64_post_inc"
	I64PreDecFunction    = "pcy_rt_i64_pre_dec"
	I64PostDecFunction   = "pcy_rt_i64_post_dec"

	// StringEqualFunction compares two Percy strings by value.
	StringEqualFunction = "pcy_rt_string_equal"

	// StringConcatFunction returns a new string that is its two arguments
	// concatenated.
	StringConcatFunction = "pcy_rt_string_concat"

	// StringAppendFunction appends to the string stored at a slot address.
	StringAppendFunction = "pcy_rt_string_append"

	// StringLengthFunction returns the number of bytes in a string.
	StringLengthFunction = "pcy_rt_string_length"

	// StringAtFunction returns the byte at an index, with a bounds check.
	StringAtFunction = "pcy_rt_string_at"

	// SubstringFunction returns a new string holding a byte range.
	SubstringFunction = "pcy_rt_substring"

	// ArrayNewFunction allocates a zero-initialized array.
	ArrayNewFunction = "pcy_rt_array_new"

	// ArrayAtFunction returns a bounds-checked pointer to an array element.
	ArrayAtFunction = "pcy_rt_array_at"

	// ArrayLengthFunction returns the number of elements in an array.
	ArrayLengthFunction = "pcy_rt_array_length"

	// ArrayPushFunction appends one element to an array.
	ArrayPushFunction = "pcy_rt_array_push"

	// SetArgsFunction stores the process argument vector before user code runs.
	SetArgsFunction = "pcy_rt_set_args"

	// ArgvFunction returns the process argument vector as an array of strings.
	ArgvFunction = "pcy_rt_argv"

	// MapNewFunction allocates an empty map with a given value size.
	MapNewFunction = "pcy_rt_map_new"

	// MapSetFunction inserts or updates a map entry.
	MapSetFunction = "pcy_rt_map_set"

	// MapAtFunction returns a pointer to a map value, failing when the key is
	// absent.
	MapAtFunction = "pcy_rt_map_at"

	// MapHasFunction reports whether a map contains a key.
	MapHasFunction = "pcy_rt_map_has"

	// MapLengthFunction returns the number of entries in a map.
	MapLengthFunction = "pcy_rt_map_length"

	// ReadFileFunction reads a whole file into a new string, fatally on failure.
	ReadFileFunction = "pcy_rt_read_file"

	// TryReadFileFunction reads a whole file and reports success or an error
	// string without terminating the program.
	TryReadFileFunction = "pcy_rt_try_read_file"

	// TryWriteFileFunction creates or truncates a file and writes a string.
	TryWriteFileFunction = "pcy_rt_try_write_file"

	// TryWriteBytesFunction creates or truncates a file and writes a byte array.
	TryWriteBytesFunction = "pcy_rt_try_write_bytes"

	// TryCreateDirectoryFunction creates one directory level.
	TryCreateDirectoryFunction = "pcy_rt_try_create_directory"

	// TryRemoveFunction removes one file, link, or empty directory.
	TryRemoveFunction = "pcy_rt_try_remove"

	// TryListDirectoryFunction lists sorted entry names.
	TryListDirectoryFunction = "pcy_rt_try_list_directory"

	// TryRunFunction launches a program and reports its status or start error.
	TryRunFunction = "pcy_rt_try_run"

	// TryCurrentDirectoryFunction returns the absolute current directory.
	TryCurrentDirectoryFunction = "pcy_rt_try_current_directory"

	// TryGetenvFunction reads one environment variable.
	TryGetenvFunction = "pcy_rt_try_getenv"

	// TryRunCaptureFunction runs a program and captures stdout/stderr.
	TryRunCaptureFunction = "pcy_rt_try_run_capture"

	// TryFileInfoFunction reports file metadata without opening the file.
	TryFileInfoFunction = "pcy_rt_try_file_info"

	// TryCreateTempDirectoryFunction creates a unique temporary directory.
	TryCreateTempDirectoryFunction = "pcy_rt_try_create_temp_directory"

	// Runtime-module raw-memory and syscall helpers.
	RawLoad8Function   = "pcy_rt_raw_load8"
	RawStore8Function  = "pcy_rt_raw_store8"
	RawLoad64Function  = "pcy_rt_raw_load64"
	RawStore64Function = "pcy_rt_raw_store64"
	RawCopyFunction    = "pcy_rt_raw_copy"
	RawZeroFunction    = "pcy_rt_raw_zero"
	SyscallFunction    = "pcy_rt_syscall"

	// StringToBytesFunction returns a string's bytes as a byte array.
	StringToBytesFunction = "pcy_rt_string_to_bytes"

	// BytesToStringFunction returns a byte array as a string.
	BytesToStringFunction = "pcy_rt_bytes_to_string"

	// FailFunction reports a fatal runtime error and terminates the program.
	FailFunction = "pcy_rt_fail"

	// ExitFunction terminates the process with a chosen status after flushing.
	ExitFunction = "pcy_rt_exit"

	// PrintIntFunction, PrintFloatFunction, PrintBoolFunction, and
	// PrintStringFunction write one value of the corresponding primitive type.
	PrintIntFunction    = "pcy_rt_print_int"
	PrintFloatFunction  = "pcy_rt_print_float"
	PrintBoolFunction   = "pcy_rt_print_bool"
	PrintStringFunction = "pcy_rt_print_string"

	// PrintSpaceFunction writes a single space between printed arguments, and
	// PrintNewlineFunction terminates a printed line.
	PrintSpaceFunction   = "pcy_rt_print_space"
	PrintNewlineFunction = "pcy_rt_print_newline"

	// EprintIntFunction, EprintFloatFunction, EprintBoolFunction, and
	// EprintStringFunction write one value of the corresponding primitive type to
	// standard error.
	EprintIntFunction    = "pcy_rt_eprint_int"
	EprintFloatFunction  = "pcy_rt_eprint_float"
	EprintBoolFunction   = "pcy_rt_eprint_bool"
	EprintStringFunction = "pcy_rt_eprint_string"

	// EprintSpaceFunction writes a single space to standard error, and
	// EprintNewlineFunction terminates a diagnostic line and flushes both streams.
	EprintSpaceFunction   = "pcy_rt_eprint_space"
	EprintNewlineFunction = "pcy_rt_eprint_newline"
)

// HeaderSource returns the C declarations shared with generated code.
func HeaderSource() string { return headerSource }

// Source returns the C implementation of the runtime.
func Source() string { return sourceSource }
