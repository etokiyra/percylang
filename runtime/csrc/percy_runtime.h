/*
 * Percy runtime: declarations shared with generated code.
 *
 * This file is part of the Percy language runtime. Generated Percy programs
 * include it, and percy_runtime.c implements it. It has no dependency on the
 * compiler's implementation language, so a future self-hosted Percy compiler
 * can use the same interface.
 *
 * The Percy runtime is licensed permissively, unlike the compiler itself, so
 * that programs compiled with Percy can be distributed under any terms the
 * author chooses. See LICENSE in this directory.
 *
 * SPDX-License-Identifier: MIT
 *
 * Copyright (c) 2026 etokiyra
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to
 * deal in the Software without restriction, including without limitation the
 * rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
 * sell copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
 * FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
 * IN THE SOFTWARE.
 */
#ifndef PERCY_RUNTIME_H
#define PERCY_RUNTIME_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#if defined(__linux__)
extern long syscall(long number, ...);
#endif

/*
 * Boehm-Demers-Weiser conservative garbage collector. Accept either the
 * <gc/gc.h> or <gc.h> layout depending on how the library was installed.
 */
#if defined(__has_include)
#  if __has_include(<gc/gc.h>)
#    include <gc/gc.h>
#  else
#    include <gc.h>
#  endif
#else
#  include <gc.h>
#endif

/*
 * Initializes the garbage collector. It must be called before any allocation.
 */
void pcy_rt_init(void);

/*
 * Allocates a zero-initialized, pointer-scanned, garbage-collected object of
 * size bytes. The collector reclaims the object automatically; Percy programs
 * never free memory by hand.
 */
void *pcy_rt_alloc(size_t size);

/*
 * An immutable Percy string: a byte length together with the bytes.
 *
 * length is the number of bytes, excluding the NUL terminator that always
 * follows them. data points at those bytes, so the representation keeps both a
 * stored length, which makes length and indexing constant-time, and
 * NUL-termination, which keeps the bytes usable with C library functions.
 *
 * A string value in generated code is a `const pcy_string *`. A null pointer is
 * the empty string, so the zero value of a string behaves as "" and an
 * unassigned string field or array element is safe to read.
 *
 * Strings are immutable, so `data` is not written through once a string is
 * observable; construction sites write the freshly allocated bytes before the
 * value can be reached. A statically known literal is a static `const
 * pcy_string` whose bytes are a static string, and a constructed string is a
 * single GC-managed object holding its header and its bytes together, so the
 * bytes can never outlive or be separated from the length that describes them.
 *
 * Embedded NUL bytes are not supported: the representation is NUL-terminated,
 * Percy provides no way to create one, and the language's string operations
 * are byte operations over NUL-free strings.
 */
typedef struct pcy_string {
    int64_t length;
    const char *data;
} pcy_string;

/*
 * Value equality for Percy strings. Comparison is by content, never by pointer
 * identity, and lengths are compared before any bytes are examined. A null
 * pointer is treated as the empty string.
 */
bool pcy_rt_string_equal(const pcy_string *left, const pcy_string *right);

/*
 * Returns a new string that is left followed by right. The result is a fresh,
 * exactly-sized, GC-managed object, so it stays valid as long as Percy code can
 * reach it and never needs a manual free. A null operand is treated as the
 * empty string, so concatenating empty strings yields an empty string.
 */
const pcy_string *pcy_rt_string_concat(const pcy_string *left, const pcy_string *right);

/*
 * Appends value to the string stored in *slot, by assigning *slot the
 * concatenation of its current value and value. It takes the address of the
 * slot so that the target is evaluated exactly once at the call site.
 */
void pcy_rt_string_append(const pcy_string **slot, const pcy_string *value);

/*
 * Returns the number of bytes in a string, not counting the terminator. Reading
 * the stored length makes this constant-time. A null pointer is the empty
 * string, so its length is 0.
 */
int64_t pcy_rt_string_length(const pcy_string *value);

/*
 * Returns the byte at index as an integer in 0..255.
 *
 * Indexing is byte-oriented, not code-point-oriented: the byte at a given byte
 * offset is returned, so a multi-byte UTF-8 character occupies several indices.
 * The stored length makes this constant-time. The index is bounds-checked; a
 * negative index, or an index at or beyond the byte length, is a fatal runtime
 * error rather than an out-of-bounds read.
 */
int64_t pcy_rt_string_at(const pcy_string *value, int64_t index);

/*
 * Returns a new string holding the bytes in [start, end).
 *
 * The range is bounds-checked and must satisfy
 * 0 <= start <= end <= byte length; any other range is a fatal runtime error.
 * The result is a fresh, exactly-sized, GC-managed object.
 */
const pcy_string *pcy_rt_substring(const pcy_string *value, int64_t start, int64_t end);

/*
 * A dynamically sized array.
 *
 * length is the number of elements and element_size is the size in bytes of one
 * element. data points to a GC-managed, zero-initialized buffer of
 * capacity * element_size bytes; length is the prefix in use. Arrays are
 * references: assigning one array to another copies the reference, and both
 * names then share the same storage, including any later growth.
 *
 * capacity is kept at the end of the struct so the fields before it keep their
 * offsets; it is an implementation detail of push and is never read directly by
 * generated code.
 */
typedef struct pcy_array {
    int64_t length;
    int64_t element_size;
    void *data;
    int64_t capacity;
} pcy_array;

/*
 * Allocates an array of length elements, each element_size bytes. Every element
 * is zero-initialized by the collector. A negative length or an allocation that
 * would overflow is a fatal runtime error.
 */
pcy_array *pcy_rt_array_new(int64_t length, int64_t element_size);

/*
 * Returns a pointer to the element at index. The index is checked against the
 * array length; a negative or out-of-range index, or a null array reference, is
 * a fatal runtime error rather than undefined behavior.
 */
void *pcy_rt_array_at(const pcy_array *array, int64_t index);

/*
 * Returns the number of elements in the array.
 */
int64_t pcy_rt_array_length(const pcy_array *array);

/*
 * Appends one element to the array and returns the new length.
 *
 * value points to element_size readable bytes, which are copied into the
 * array; the caller's storage is not retained. When the array is full its data
 * buffer is replaced by one twice as large (at least one element), the live
 * elements are copied, and the old buffer becomes garbage for the collector.
 * Because the array object itself keeps its identity, every reference to the
 * array observes the append. A null array reference, an element size that
 * cannot be represented, or an allocation that would overflow is a fatal
 * runtime error.
 */
int64_t pcy_rt_array_push(pcy_array *array, const void *value);

/*
 * Returns a fresh byte array containing the bytes of value. The result uses the
 * ordinary array representation with an element size of one; a null string is
 * the empty string. The string's terminator is not part of the result.
 */
pcy_array *pcy_rt_string_to_bytes(const pcy_string *value);

/*
 * Returns a fresh string containing the bytes of a byte array. A null array or
 * an array whose element size is not one is a fatal runtime error. A NUL byte
 * anywhere in the array is a fatal runtime error naming its index, because
 * Percy strings cannot contain NUL.
 */
const pcy_string *pcy_rt_bytes_to_string(const pcy_array *bytes);

/*
 * Stores the process argument vector for a later pcy_rt_argv call. The
 * generated C entry point calls this once, before the Percy main function
 * runs. The pointers remain valid for the lifetime of the process.
 */
void pcy_rt_set_args(int argc, char **argv);

/*
 * Returns a new pcy_array of const pcy_string * holding the process argument
 * vector, including the program name at index 0. Every string is a fresh
 * GC-managed copy. Each call returns a fresh array, so a program that mutates
 * the array does not change the process arguments or a later call's result.
 * Before pcy_rt_set_args the argument count is zero.
 */
pcy_array *pcy_rt_argv(void);

/*
 * A map from string keys to values of a fixed size.
 *
 * length is the number of entries; capacity is the number of slots in the hash
 * table, always a power of two or zero; value_size is the size in bytes of one
 * value. keys, hashes, and values are GC-managed arrays of capacity slots. keys
 * holds pointers to strings, so a null entry marks an empty slot and stored keys
 * are never null; because the array is pointer-scanned, a stored key stays alive
 * as long as the map does. Maps are references: assigning one map to another
 * copies the reference, and both names then share the same table.
 */
typedef struct pcy_map {
    int64_t length;
    int64_t capacity;
    int64_t value_size;
    const pcy_string **keys;
    uint64_t *hashes;
    void *values;
} pcy_map;

/*
 * Allocates an empty map whose values are value_size bytes each. Keys are
 * compared by string content, so two distinct strings with the same contents
 * address the same entry. Map allocation uses the garbage collector.
 */
pcy_map *pcy_rt_map_new(int64_t value_size);

/*
 * Stores value under key, inserting the entry when the key is absent and
 * updating it when the key is present. value points to value_size bytes to copy
 * into the entry, so a map can hold any value type without type punning. The map
 * retains the key, not a copy of it, so the key stays alive through the map.
 */
void pcy_rt_map_set(pcy_map *map, const pcy_string *key, const void *value);

/*
 * Returns a pointer to the value stored under key. Looking up a key that is
 * absent, or using a null map reference, is a fatal runtime error rather than
 * undefined behavior or a fabricated value.
 */
void *pcy_rt_map_at(const pcy_map *map, const pcy_string *key);

/*
 * Reports whether key is present in the map.
 */
bool pcy_rt_map_has(const pcy_map *map, const pcy_string *key);

/*
 * Returns the number of entries in the map.
 */
int64_t pcy_rt_map_length(const pcy_map *map);

/*
 * The largest file, in bytes, that pcy_rt_read_file will read. Larger files are
 * rejected instead of allocated, so a program cannot be made to allocate an
 * unbounded buffer from a file on disk.
 */
#define PCY_RT_MAX_FILE_SIZE ((size_t)256 * 1024 * 1024)

/*
 * Reports an unrecoverable runtime error to standard error and terminates the
 * program with a nonzero exit status. It is used by the runtime itself and by
 * generated code, for example when a match inspects an invalid variant tag or a
 * null union reference.
 *
 * It never returns, which is declared so that generated switches with a
 * fail-fast default case do not warn about falling off the end of a function.
 */
_Noreturn void pcy_rt_fail(const char *message);

/*
 * Raw-memory and syscall intrinsics for the trusted <runtime> module. These
 * helpers deliberately bypass Percy's ordinary safety guarantees: addresses
 * are trusted and never validated. They are static inline so generated code
 * needs no extra object, and the explicit memcpy/memmove/memset operations avoid
 * alignment and strict-aliasing undefined behavior in the generated C.
 */
static inline void pcy_rt_raw_check_size(int64_t size) {
    if (size < 0) {
        char message[64];
        snprintf(message, sizeof(message), "negative size %lld", (long long)size);
        pcy_rt_fail(message);
    }
}

static inline uint8_t pcy_rt_raw_load8(int64_t address) {
    uint8_t value;
    memcpy(&value, (const void *)(uintptr_t)address, sizeof(value));
    return value;
}

static inline void pcy_rt_raw_store8(int64_t address, uint8_t value) {
    memcpy((void *)(uintptr_t)address, &value, sizeof(value));
}

static inline int64_t pcy_rt_raw_load64(int64_t address) {
    int64_t value;
    memcpy(&value, (const void *)(uintptr_t)address, sizeof(value));
    return value;
}

static inline void pcy_rt_raw_store64(int64_t address, int64_t value) {
    memcpy((void *)(uintptr_t)address, &value, sizeof(value));
}

static inline void pcy_rt_raw_copy(int64_t destination, int64_t source, int64_t size) {
    pcy_rt_raw_check_size(size);
    memmove((void *)(uintptr_t)destination, (const void *)(uintptr_t)source, (size_t)size);
}

static inline void pcy_rt_raw_zero(int64_t address, int64_t size) {
    pcy_rt_raw_check_size(size);
    memset((void *)(uintptr_t)address, 0, (size_t)size);
}

static inline int64_t pcy_rt_syscall(int64_t number, int64_t a, int64_t b, int64_t c, int64_t d, int64_t e, int64_t f) {
#if defined(__linux__)
    long result = syscall((long)number, (long)a, (long)b, (long)c, (long)d, (long)e, (long)f);
    return (int64_t)result;
#else
    (void)number; (void)a; (void)b; (void)c; (void)d; (void)e; (void)f;
    pcy_rt_fail("syscall is not supported on this platform");
#endif
}

/*
 * Defined 64-bit integer arithmetic. Percy's int is a signed 64-bit
 * two's-complement value, and these helpers compute with unsigned bit patterns
 * so generated C never evaluates signed overflow. Division by zero is a fatal
 * runtime error; INT64_MIN / -1 wraps to INT64_MIN and INT64_MIN % -1 is zero.
 * The assignment helpers take the address of the target so the target is
 * evaluated exactly once. These are static inline because every generated unit
 * that does integer arithmetic includes this header.
 */
static inline int64_t pcy_rt_i64_from_bits(uint64_t bits) {
    int64_t value;
    memcpy(&value, &bits, sizeof value);
    return value;
}

static inline uint64_t pcy_rt_i64_bits(int64_t value) {
    uint64_t bits;
    memcpy(&bits, &value, sizeof bits);
    return bits;
}

static inline int64_t pcy_rt_i64_add(int64_t left, int64_t right) {
    return pcy_rt_i64_from_bits(pcy_rt_i64_bits(left) + pcy_rt_i64_bits(right));
}

static inline int64_t pcy_rt_i64_sub(int64_t left, int64_t right) {
    return pcy_rt_i64_from_bits(pcy_rt_i64_bits(left) - pcy_rt_i64_bits(right));
}

static inline int64_t pcy_rt_i64_mul(int64_t left, int64_t right) {
    return pcy_rt_i64_from_bits(pcy_rt_i64_bits(left) * pcy_rt_i64_bits(right));
}

static inline int64_t pcy_rt_i64_neg(int64_t value) {
    return pcy_rt_i64_from_bits(UINT64_C(0) - pcy_rt_i64_bits(value));
}

static inline int64_t pcy_rt_i64_div(int64_t left, int64_t right) {
    if (right == 0) {
        pcy_rt_fail("integer division by zero");
    }
    if (left == INT64_MIN && right == -1) {
        return INT64_MIN;
    }
    return left / right;
}

static inline int64_t pcy_rt_i64_mod(int64_t left, int64_t right) {
    if (right == 0) {
        pcy_rt_fail("integer division by zero");
    }
    if (left == INT64_MIN && right == -1) {
        return 0;
    }
    return left % right;
}

static inline void pcy_rt_i64_check_shift(int64_t count) {
    if (count < 0 || count > 63) {
        char message[128];
        snprintf(message, sizeof message, "shift count %lld is outside 0..63", (long long)count);
        pcy_rt_fail(message);
    }
}

static inline int64_t pcy_rt_i64_shl(int64_t left, int64_t count) {
    pcy_rt_i64_check_shift(count);
    return pcy_rt_i64_from_bits(pcy_rt_i64_bits(left) << (unsigned)count);
}

static inline int64_t pcy_rt_i64_shr(int64_t left, int64_t count) {
    pcy_rt_i64_check_shift(count);
    uint64_t bits = pcy_rt_i64_bits(left);
    uint64_t shifted;
    if ((bits & (UINT64_C(1) << 63)) == 0) {
        shifted = bits >> (unsigned)count;
    } else {
        shifted = ~(~bits >> (unsigned)count);
    }
    return pcy_rt_i64_from_bits(shifted);
}

static inline int64_t pcy_rt_i64_add_assign(int64_t *slot, int64_t value) {
    *slot = pcy_rt_i64_add(*slot, value);
    return *slot;
}

static inline int64_t pcy_rt_i64_sub_assign(int64_t *slot, int64_t value) {
    *slot = pcy_rt_i64_sub(*slot, value);
    return *slot;
}

static inline int64_t pcy_rt_i64_pre_inc(int64_t *slot) {
    *slot = pcy_rt_i64_add(*slot, 1);
    return *slot;
}

static inline int64_t pcy_rt_i64_post_inc(int64_t *slot) {
    int64_t previous = *slot;
    *slot = pcy_rt_i64_add(previous, 1);
    return previous;
}

static inline int64_t pcy_rt_i64_pre_dec(int64_t *slot) {
    *slot = pcy_rt_i64_sub(*slot, 1);
    return *slot;
}

static inline int64_t pcy_rt_i64_post_dec(int64_t *slot) {
    int64_t previous = *slot;
    *slot = pcy_rt_i64_sub(previous, 1);
    return previous;
}

/*
 * Flushes standard output and then standard error and terminates the process
 * with status. It does not validate or convert the status: on POSIX systems only
 * the low eight bits are observable, so 256 is observed as 0 and -1 as 255. The
 * failing runtime path uses the same flush sequence before exiting with status 1.
 */
_Noreturn void pcy_rt_exit(int64_t status);

/*
 * Reads the entire contents of the regular file at path into a new,
 * GC-managed string and returns it. An empty file yields the empty string.
 *
 * The path is a Percy string. Failure is fatal: a missing or unreadable file,
 * a path that is not a seekable regular file, a file larger than
 * PCY_RT_MAX_FILE_SIZE, or a read error terminates the program with a message
 * on standard error and a nonzero exit status. The file is always closed.
 *
 * Because strings are NUL-terminated, content after the first NUL byte in the
 * file is not represented. The returned string is a snapshot of the file at the
 * time of the read; if the file shrinks while it is being read, fewer bytes are
 * returned and the stored length is adjusted to match.
 */
const pcy_string *pcy_rt_read_file(const pcy_string *path);

/*
 * Reads the entire contents of the regular file at path without terminating on
 * a file error. On success it returns true, stores a fresh GC-managed string in
 * *text, and stores NULL in *message. On failure it returns false, stores NULL
 * in *text, and stores a fresh GC-managed Go-compatible OS error message in
 * *message (for example "open /x: no such file or directory"). The caller's
 * ReadFileResult helper adds no prefix; the loader may add
 * "cannot read source file: ". The same PCY_RT_MAX_FILE_SIZE limit as
 * pcy_rt_read_file applies; an oversized file is a failure, not a crash.
 */
bool pcy_rt_try_read_file(const pcy_string *path,
                          const pcy_string **text,
                          const pcy_string **message);

/*
 * Recoverable write and directory operations. Each returns true on success and
 * stores NULL in *message; on failure it returns false and stores a fresh
 * GC-managed Go-compatible error message. Every descriptor is closed on every
 * path. The write functions create or truncate with mode 0644, never append,
 * and never create parent directories.
 */
bool pcy_rt_try_write_file(const pcy_string *path,
                          const pcy_string *text,
                          const pcy_string **message);
bool pcy_rt_try_write_bytes(const pcy_string *path,
                           const pcy_array *data,
                           const pcy_string **message);
bool pcy_rt_try_create_directory(const pcy_string *path,
                                 const pcy_string **message);
bool pcy_rt_try_remove(const pcy_string *path,
                       const pcy_string **message);

/*
 * Reads one directory into a fresh array of strings, sorted bytewise ascending
 * and excluding "." and "..". On success *entries is the array and *message is
 * NULL; on failure *entries is NULL and *message holds the error.
 */
bool pcy_rt_try_list_directory(const pcy_string *path,
                               pcy_array **entries,
                               const pcy_string **message);

/*
 * Executes program directly with execve semantics, using args as argv[1..]
 * and program as argv[0]. The child inherits the environment, working
 * directory, and descriptors 0, 1, and 2. On success the child's exit status is
 * stored in *status (128 + signal for a signal termination) and *message is
 * NULL. A failure to start the program stores a Go-compatible
 * "fork/exec <program>: <reason>" message and returns false; a nonzero child
 * status is still success. Both output streams are flushed before forking.
 */
bool pcy_rt_try_run(const pcy_string *program,
                    const pcy_array *args,
                    int64_t *status,
                    const pcy_string **message);

/*
 * Returns the process's absolute current working directory as a fresh
 * GC-managed string. It uses getcwd, ignores PWD, and grows the buffer on
 * ERANGE. On failure it stores a Go-compatible "getwd: <reason>" message and
 * returns false.
 */
bool pcy_rt_try_current_directory(const pcy_string **path,
                                  const pcy_string **message);

/*
 * Reads one environment variable. Returns true and stores a fresh GC-managed
 * copy of its value when the variable exists, including when the value is the
 * empty string. Returns false when the variable is unset, the name is empty, or
 * the name contains '='. PWD is an ordinary variable here; no libc ordering or
 * shell expansion is involved.
 */
bool pcy_rt_try_getenv(const pcy_string *name, const pcy_string **value);

/*
 * Runs program with try_run's direct-exec semantics while putting stdin on
 * /dev/null and capturing stdout and stderr separately. Each stream is capped
 * at 16777216 bytes; exceeding the cap kills and reaps the child and fails with
 * "run <program>: <stream> exceeds the 16777216 byte limit". A NUL byte in a
 * captured stream fails after reaping with
 * "run <program>: <stream> contains a NUL byte at offset <n>".
 */
bool pcy_rt_try_run_capture(const pcy_string *program,
                            const pcy_array *args,
                            int64_t *status,
                            const pcy_string **out,
                            const pcy_string **err,
                            const pcy_string **message);

/* Result codes for pcy_rt_try_file_info. */
#define PCY_RT_FILE_INFO_OK 0
#define PCY_RT_FILE_INFO_MISSING 1
#define PCY_RT_FILE_INFO_FAILED 2

/*
 * Reports file metadata without opening a file descriptor. lstat classifies the
 * path itself; a symlink is followed by stat for is_file/is_directory/size, and
 * access(X_OK) supplies executable. ENOENT/ENOTDIR from lstat is Missing; a
 * dangling link is Ok with only is_symlink set. Other failures use lstat/stat
 * path text. Sizes above INT64_MAX cannot occur.
 */
int pcy_rt_try_file_info(const pcy_string *path,
                         bool *is_file,
                         bool *is_directory,
                         bool *is_symlink,
                         bool *executable,
                         int64_t *size,
                         const pcy_string **message);

/*
 * Creates an absolute, unique temporary directory. The base is a non-empty
 * $TMPDIR (made absolute against getcwd when relative) or /tmp; an unusable
 * base fails instead of falling back. The prefix must be 1..64 ASCII bytes from
 * [A-Za-z0-9._-], and may not be "..", or the call fails with
 * "invalid temporary directory prefix" before filesystem access. mkdtemp
 * supplies the atomic six-character suffix and mode 0700. Returns true and a
 * fresh GC-managed path, or false and a Go-compatible mkdir/stat message.
 * Opens no file descriptor and registers no cleanup hook.
 */
bool pcy_rt_try_create_temp_directory(const pcy_string *prefix,
                                      const pcy_string **path,
                                      const pcy_string **message);

/*
 * Built-in print helpers. Each writes a single value. Callers separate
 * arguments with single spaces and terminate the line with
 * pcy_rt_print_newline.
 */
void pcy_rt_print_int(int64_t value);
void pcy_rt_print_float(double value);
void pcy_rt_print_bool(bool value);
void pcy_rt_print_string(const pcy_string *value);
void pcy_rt_print_space(void);
void pcy_rt_print_newline(void);

/*
 * Built-in eprint helpers. Each writes a single value to standard error, using
 * the same formatting as the print helper for that type. Callers separate
 * arguments with pcy_rt_eprint_space and terminate the line with
 * pcy_rt_eprint_newline. The newline helper flushes standard output before
 * writing the newline, so earlier print output appears first on a shared
 * terminal, and then flushes standard error.
 */
void pcy_rt_eprint_int(int64_t value);
void pcy_rt_eprint_float(double value);
void pcy_rt_eprint_bool(bool value);
void pcy_rt_eprint_string(const pcy_string *value);
void pcy_rt_eprint_space(void);
void pcy_rt_eprint_newline(void);

#endif /* PERCY_RUNTIME_H */
