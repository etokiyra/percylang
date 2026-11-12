/*
 * Percy runtime: implementation. See percy_runtime.h for the interface.
 *
 * This file is part of the Percy language runtime, which is compiled into
 * every generated Percy program.
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
#define _POSIX_C_SOURCE 200809L

#include "percy_runtime.h"

extern char **environ;

#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>

/*
 * Flushes both output streams in the order a process exit must preserve: stdout
 * first, then stderr. Every terminating path shares this sequence.
 */
static void pcy_rt_flush_streams(void) {
    fflush(stdout);
    fflush(stderr);
}

/*
 * Reports an unrecoverable runtime error to standard error and terminates the
 * program. Used for conditions that cannot be recovered from, such as an
 * out-of-range array index, an allocation that would overflow, or inspecting a
 * null union reference.
 */
void pcy_rt_fail(const char *message) {
    pcy_rt_flush_streams();
    fputs("purr: ", stderr);
    fputs(message, stderr);
    fputc('\n', stderr);
    pcy_rt_exit(1);
}

void pcy_rt_exit(int64_t status) {
    pcy_rt_flush_streams();
    exit((int)status);
}

/*
 * Reports an unrecoverable error that concerns a path, including the operating
 * system message for a saved errno value.
 */
static void pcy_rt_fail_path(const char *action, const pcy_string *path, int error) {
    char message[1024];
    snprintf(message, sizeof(message), "%s '%s': %s", action, path->data, strerror(error));
    pcy_rt_fail(message);
}

void pcy_rt_init(void) {
    GC_INIT();
}

void *pcy_rt_alloc(size_t size) {
    return GC_MALLOC(size);
}

/*
 * The static empty string. The zero value of a string is a null pointer, so
 * normalization maps every null string to this object, whose data is the static
 * empty C string. This makes unassigned string array elements and struct fields
 * safe to read.
 */
static const pcy_string pcy_rt_empty_string = { 0, "" };

/*
 * Returns value, or the static empty string when value is null.
 *
 * Every runtime function that reads a string normalizes first, so no caller has
 * to check for a null reference and no function has to read through a null
 * pointer.
 */
static const pcy_string *pcy_rt_string_value(const pcy_string *value) {
    return value == NULL ? &pcy_rt_empty_string : value;
}

/*
 * Allocates a new string of exactly length bytes and returns it.
 *
 * The header is fully initialized: length is set, data points at the length
 * bytes that immediately follow it, and a NUL terminator is written one past
 * them, so the value is always a valid C string even while it is being filled.
 * The collector zeroes the allocation, so the bytes are a defined all-zero
 * sequence rather than indeterminate storage, and the bytes are written by the
 * caller through pcy_rt_string_bytes before the string becomes observable.
 */
static pcy_string *pcy_rt_string_allocate(int64_t length) {
    if (length < 0) {
        pcy_rt_fail("string length cannot be negative");
    }
    if ((uint64_t)length > (uint64_t)SIZE_MAX - sizeof(pcy_string) - 1) {
        pcy_rt_fail("string is too large");
    }

    pcy_string *result = (pcy_string *)pcy_rt_alloc(sizeof(pcy_string) + (size_t)length + 1);
    result->length = length;
    char *bytes = (char *)(result + 1);
    bytes[length] = '\0';
    result->data = bytes;
    return result;
}

/*
 * Returns the bytes of a string that was just allocated, for filling in.
 *
 * A string's bytes are immutable once the string is observable, but the bytes
 * of a freshly allocated object are written during construction. Only
 * construction sites, which hold the object just returned by
 * pcy_rt_string_allocate, call this.
 */
static char *pcy_rt_string_bytes(pcy_string *value) {
    return (char *)value->data;
}

bool pcy_rt_string_equal(const pcy_string *left, const pcy_string *right) {
    left = pcy_rt_string_value(left);
    right = pcy_rt_string_value(right);

    if (left->length != right->length) {
        return false;
    }
    if (left->length == 0) {
        return true;
    }
    return memcmp(left->data, right->data, (size_t)left->length) == 0;
}

const pcy_string *pcy_rt_string_concat(const pcy_string *left, const pcy_string *right) {
    left = pcy_rt_string_value(left);
    right = pcy_rt_string_value(right);

    /*
     * Guard the combined length against overflow. Two strings this large cannot
     * both exist in memory, so this is defensive, but it keeps the size
     * computation well defined instead of allocating too little.
     */
    if (left->length > INT64_MAX - right->length) {
        pcy_rt_fail("string concatenation result is too large");
    }

    int64_t length = left->length + right->length;
    pcy_string *result = pcy_rt_string_allocate(length);
    char *bytes = pcy_rt_string_bytes(result);
    memcpy(bytes, left->data, (size_t)left->length);
    memcpy(bytes + left->length, right->data, (size_t)right->length);
    return result;
}

void pcy_rt_string_append(const pcy_string **slot, const pcy_string *value) {
    *slot = pcy_rt_string_concat(*slot, value);
}

int64_t pcy_rt_string_length(const pcy_string *value) {
    return pcy_rt_string_value(value)->length;
}

int64_t pcy_rt_string_at(const pcy_string *value, int64_t index) {
    const pcy_string *checked = pcy_rt_string_value(value);
    if (index < 0 || index >= checked->length) {
        char message[128];
        snprintf(message, sizeof(message), "string index %lld is out of range for length %lld", (long long)index, (long long)checked->length);
        pcy_rt_fail(message);
    }
    return (int64_t)(unsigned char)checked->data[index];
}

const pcy_string *pcy_rt_substring(const pcy_string *value, int64_t start, int64_t end) {
    const pcy_string *checked = pcy_rt_string_value(value);
    if (start < 0 || end < start || end > checked->length) {
        char message[160];
        snprintf(message, sizeof(message), "substring range %lld..%lld is out of range for length %lld", (long long)start, (long long)end, (long long)checked->length);
        pcy_rt_fail(message);
    }

    int64_t count = end - start;
    pcy_string *result = pcy_rt_string_allocate(count);
    memcpy(pcy_rt_string_bytes(result), checked->data + start, (size_t)count);
    return result;
}

pcy_array *pcy_rt_array_new(int64_t length, int64_t element_size) {
    if (length < 0) {
        pcy_rt_fail("array length cannot be negative");
    }
    if (element_size <= 0) {
        pcy_rt_fail("array element size must be positive");
    }
    if ((uint64_t)length > (uint64_t)SIZE_MAX / (uint64_t)element_size) {
        pcy_rt_fail("array allocation is too large");
    }

    pcy_array *array = (pcy_array *)pcy_rt_alloc(sizeof(pcy_array));
    array->length = length;
    array->element_size = element_size;
    array->capacity = length;

    size_t bytes = (size_t)length * (size_t)element_size;
    if (bytes == 0) {
        /* Never make a zero-size allocation; an empty array is still a valid
         * object with a non-null data pointer. */
        bytes = 1;
    }
    array->data = pcy_rt_alloc(bytes);
    return array;
}

void *pcy_rt_array_at(const pcy_array *array, int64_t index) {
    if (array == NULL) {
        pcy_rt_fail("array access on a null array reference");
    }
    if (index < 0 || index >= array->length) {
        char message[128];
        snprintf(message, sizeof(message), "array index %lld is out of range for length %lld", (long long)index, (long long)array->length);
        pcy_rt_fail(message);
    }
    return (char *)array->data + (size_t)index * (size_t)array->element_size;
}

int64_t pcy_rt_array_length(const pcy_array *array) {
    if (array == NULL) {
        pcy_rt_fail("length of a null array reference");
    }
    return array->length;
}

int64_t pcy_rt_array_push(pcy_array *array, const void *value) {
    if (array == NULL) {
        pcy_rt_fail("append to a null array reference");
    }
    if (value == NULL) {
        pcy_rt_fail("append of a null element pointer");
    }
    if (array->length < 0 || array->element_size <= 0 || array->capacity < array->length) {
        pcy_rt_fail("array is corrupt");
    }

    if (array->length == array->capacity) {
        int64_t capacity = array->capacity;
        if (capacity < 1) {
            capacity = 1;
        } else {
            if (capacity > INT64_MAX / 2) {
                pcy_rt_fail("array allocation is too large");
            }
            capacity = capacity * 2;
        }
        if ((uint64_t)capacity > (uint64_t)SIZE_MAX / (uint64_t)array->element_size) {
            pcy_rt_fail("array allocation is too large");
        }
        void *data = pcy_rt_alloc((size_t)capacity * (size_t)array->element_size);
        if (array->length > 0) {
            memcpy(data, array->data, (size_t)array->length * (size_t)array->element_size);
        }
        array->data = data;
        array->capacity = capacity;
    }

    memcpy((char *)array->data + (size_t)array->length * (size_t)array->element_size, value, (size_t)array->element_size);
    array->length = array->length + 1;
    return array->length;
}

pcy_array *pcy_rt_string_to_bytes(const pcy_string *value) {
    const pcy_string *checked = pcy_rt_string_value(value);
    pcy_array *result = pcy_rt_array_new(checked->length, (int64_t)sizeof(uint8_t));
    memcpy(result->data, checked->data, (size_t)checked->length);
    return result;
}

const pcy_string *pcy_rt_bytes_to_string(const pcy_array *bytes) {
    if (bytes == NULL) {
        pcy_rt_fail("byte array is null");
    }
    if (bytes->element_size != (int64_t)sizeof(uint8_t)) {
        pcy_rt_fail("byte array element size is not 1");
    }

    const uint8_t *data = (const uint8_t *)bytes->data;
    for (int64_t index = 0; index < bytes->length; index++) {
        if (data[index] == 0) {
            char message[160];
            snprintf(message, sizeof(message), "byte array contains a NUL byte at index %lld", (long long)index);
            pcy_rt_fail(message);
        }
    }

    pcy_string *result = pcy_rt_string_allocate(bytes->length);
    memcpy(pcy_rt_string_bytes(result), data, (size_t)bytes->length);
    return result;
}

/*
 * The process argument vector saved by pcy_rt_set_args. It is initialized to an
 * empty vector so that pcy_rt_argv is safe before the generated entry point has
 * run, although generated programs always set it before user code.
 */
static int pcy_rt_saved_argc = 0;
static char **pcy_rt_saved_argv = NULL;

void pcy_rt_set_args(int argc, char **argv) {
    pcy_rt_saved_argc = argc < 0 ? 0 : argc;
    pcy_rt_saved_argv = argv;
}

pcy_array *pcy_rt_argv(void) {
    int count = pcy_rt_saved_argc;
    pcy_array *result = pcy_rt_array_new((int64_t)count, sizeof(const pcy_string *));
    const pcy_string **slots = (const pcy_string **)result->data;
    int index = 0;
    while (index < count) {
        const char *text = "";
        if (pcy_rt_saved_argv != NULL && pcy_rt_saved_argv[index] != NULL) {
            text = pcy_rt_saved_argv[index];
        }
        size_t length = strlen(text);
        pcy_string *value = pcy_rt_string_allocate((int64_t)length);
        memcpy(pcy_rt_string_bytes(value), text, length);
        slots[index] = value;
        index++;
    }
    return result;
}

/*
 * FNV-1a hash of a string, computed over its bytes so that two distinct strings
 * with the same contents hash identically.
 */
static uint64_t pcy_rt_string_hash(const pcy_string *value) {
    uint64_t hash = 14695981039346656037ULL; /* FNV-1a 64-bit offset basis */
    const unsigned char *bytes = (const unsigned char *)value->data;
    for (int64_t index = 0; index < value->length; index++) {
        hash ^= (uint64_t)bytes[index];
        hash *= 1099511628211ULL; /* FNV-1a 64-bit prime */
    }
    return hash;
}

/*
 * Returns the index of the slot holding key, or -1 when the key is absent.
 *
 * The table uses open addressing with linear probing over a power-of-two
 * capacity. Entries are never removed, so the first empty slot proves the key
 * is absent. The probe is bounded by the capacity so that a corrupted table
 * cannot loop forever.
 */
static int64_t pcy_rt_map_find(const pcy_map *map, const pcy_string *key, uint64_t hash) {
    if (map->capacity == 0) {
        return -1;
    }
    size_t mask = (size_t)map->capacity - 1;
    size_t index = (size_t)hash & mask;
    for (int64_t probes = 0; probes < map->capacity; probes++) {
        const pcy_string *existing = map->keys[index];
        if (existing == NULL) {
            return -1;
        }
        if (map->hashes[index] == hash && pcy_rt_string_equal(existing, key)) {
            return (int64_t)index;
        }
        index = (index + 1) & mask;
    }
    return -1;
}

/* Stores key and a copy of the value's bytes into the first empty slot. */
static void pcy_rt_map_place(pcy_map *map, const pcy_string *key, uint64_t hash, const void *value) {
    size_t mask = (size_t)map->capacity - 1;
    size_t index = (size_t)hash & mask;
    for (int64_t probes = 0; probes < map->capacity; probes++) {
        if (map->keys[index] == NULL) {
            map->keys[index] = key;
            map->hashes[index] = hash;
            memcpy((char *)map->values + index * (size_t)map->value_size, value, (size_t)map->value_size);
            map->length++;
            return;
        }
        index = (index + 1) & mask;
    }
    pcy_rt_fail("map is full");
}

/*
 * Grows the table when inserting one more entry would exceed a 3/4 load factor.
 * All existing entries are rehashed into the new table, so no entry is lost.
 */
static void pcy_rt_map_reserve(pcy_map *map) {
    if (map->capacity != 0 && (map->length + 1) * 4 <= map->capacity * 3) {
        return;
    }
    if (map->capacity > INT64_MAX / 2) {
        pcy_rt_fail("map is too large");
    }
    int64_t capacity = map->capacity == 0 ? 8 : map->capacity * 2;

    if ((uint64_t)capacity > (uint64_t)SIZE_MAX / sizeof(const pcy_string *) ||
        (uint64_t)capacity > (uint64_t)SIZE_MAX / sizeof(uint64_t) ||
        (uint64_t)capacity > (uint64_t)SIZE_MAX / (uint64_t)map->value_size) {
        pcy_rt_fail("map allocation is too large");
    }

    /* Keep the old table reachable while the new one is built, so the collector
     * does not reclaim keys or pointer values that are still being copied. */
    const pcy_string **old_keys = map->keys;
    uint64_t *old_hashes = map->hashes;
    void *old_values = map->values;
    int64_t old_capacity = map->capacity;

    map->capacity = capacity;
    map->length = 0;
    map->keys = (const pcy_string **)pcy_rt_alloc((size_t)capacity * sizeof(const pcy_string *));
    map->hashes = (uint64_t *)pcy_rt_alloc((size_t)capacity * sizeof(uint64_t));
    map->values = pcy_rt_alloc((size_t)capacity * (size_t)map->value_size);

    for (int64_t index = 0; index < old_capacity; index++) {
        if (old_keys[index] != NULL) {
            pcy_rt_map_place(map, old_keys[index], old_hashes[index],
                             (const char *)old_values + (size_t)index * (size_t)map->value_size);
        }
    }
}

pcy_map *pcy_rt_map_new(int64_t value_size) {
    if (value_size <= 0) {
        pcy_rt_fail("map value size must be positive");
    }
    pcy_map *map = (pcy_map *)pcy_rt_alloc(sizeof(pcy_map));
    map->length = 0;
    map->capacity = 0;
    map->value_size = value_size;
    map->keys = NULL;
    map->hashes = NULL;
    map->values = NULL;
    return map;
}

void pcy_rt_map_set(pcy_map *map, const pcy_string *key, const void *value) {
    if (map == NULL) {
        pcy_rt_fail("map assignment on a null map reference");
    }
    if (value == NULL) {
        pcy_rt_fail("map value is missing");
    }

    /* Normalize a null key to the static empty string so that stored keys are
     * never null and an empty key can be distinguished from an empty slot. */
    const pcy_string *checked = pcy_rt_string_value(key);
    uint64_t hash = pcy_rt_string_hash(checked);

    int64_t index = pcy_rt_map_find(map, checked, hash);
    if (index >= 0) {
        memcpy((char *)map->values + (size_t)index * (size_t)map->value_size, value, (size_t)map->value_size);
        return;
    }

    pcy_rt_map_reserve(map);
    pcy_rt_map_place(map, checked, hash, value);
}

void *pcy_rt_map_at(const pcy_map *map, const pcy_string *key) {
    if (map == NULL) {
        pcy_rt_fail("map lookup on a null map reference");
    }
    const pcy_string *checked = pcy_rt_string_value(key);
    int64_t index = pcy_rt_map_find(map, checked, pcy_rt_string_hash(checked));
    if (index < 0) {
        char message[256];
        snprintf(message, sizeof(message), "map has no entry for key \"%.*s\"", 160, checked->data);
        pcy_rt_fail(message);
    }
    return (char *)map->values + (size_t)index * (size_t)map->value_size;
}

bool pcy_rt_map_has(const pcy_map *map, const pcy_string *key) {
    if (map == NULL) {
        pcy_rt_fail("has on a null map reference");
    }
    const pcy_string *checked = pcy_rt_string_value(key);
    return pcy_rt_map_find(map, checked, pcy_rt_string_hash(checked)) >= 0;
}

int64_t pcy_rt_map_length(const pcy_map *map) {
    if (map == NULL) {
        pcy_rt_fail("length of a null map reference");
    }
    return map->length;
}

/*
 * Returns the Go-style text for a saved errno. The common cases are explicit
 * because Go uses lower-case operation/error text rather than strerror's
 * capitalized, locale-dependent text.
 */
static const char *pcy_rt_go_error_reason(int error, char *fallback, size_t capacity) {
    switch (error) {
        case ENOENT:
            return "no such file or directory";
        case EACCES:
            return "permission denied";
        case EISDIR:
            return "is a directory";
        case ENOTDIR:
            return "not a directory";
        case ENAMETOOLONG:
            return "file name too long";
        case EMFILE:
            return "too many open files";
        case ENFILE:
            return "too many open files in system";
        case ELOOP:
            return "too many levels of symbolic links";
        case EEXIST:
            return "file exists";
        case ENOTEMPTY:
            return "directory not empty";
        default:
            break;
    }
    snprintf(fallback, capacity, "%s", strerror(error));
    if (fallback[0] >= 'A' && fallback[0] <= 'Z') {
        fallback[0] = (char)(fallback[0] + ('a' - 'A'));
    }
    return fallback;
}

/*
 * Builds "<operation> <path>: <reason>" as a GC-managed string. The path is the
 * exact Percy path bytes; no quotes are added, matching Go's os.ReadFile text.
 */
static const pcy_string *pcy_rt_path_error_with_reason(const char *operation, const pcy_string *path, const char *reason) {
    size_t operation_length = strlen(operation);
    size_t reason_length = strlen(reason);
    size_t length = operation_length + 1 + (size_t)path->length + 2 + reason_length;
    pcy_string *result = pcy_rt_string_allocate((int64_t)length);
    char *bytes = pcy_rt_string_bytes(result);
    snprintf(bytes, length + 1, "%s %s: %s", operation, path->data, reason);
    return result;
}

static const pcy_string *pcy_rt_path_error(const char *operation, const pcy_string *path, int error) {
    char fallback[256];
    const char *reason = pcy_rt_go_error_reason(error, fallback, sizeof(fallback));
    return pcy_rt_path_error_with_reason(operation, path, reason);
}

/* Builds "<operation>: <reason>" for an operation with no path operand. */
static const pcy_string *pcy_rt_operation_error(const char *operation, int error) {
    char fallback[256];
    const char *reason = pcy_rt_go_error_reason(error, fallback, sizeof(fallback));
    size_t operation_length = strlen(operation);
    size_t reason_length = strlen(reason);
    size_t length = operation_length + 2 + reason_length;
    pcy_string *result = pcy_rt_string_allocate((int64_t)length);
    char *bytes = pcy_rt_string_bytes(result);
    snprintf(bytes, length + 1, "%s: %s", operation, reason);
    return result;
}

/* Returns a plain failure message string for a non-path condition. */
static const pcy_string *pcy_rt_plain_error(const char *message) {
    size_t length = strlen(message);
    pcy_string *result = pcy_rt_string_allocate((int64_t)length);
    memcpy(pcy_rt_string_bytes(result), message, length);
    return result;
}

bool pcy_rt_try_read_file(const pcy_string *path, const pcy_string **text, const pcy_string **message) {
    if (text == NULL || message == NULL) {
        pcy_rt_fail("try_read_file received a null result slot");
    }
    *text = NULL;
    *message = NULL;

    const pcy_string *checked = pcy_rt_string_value(path);
    int descriptor = open(checked->data, O_RDONLY);
    if (descriptor < 0) {
        *message = pcy_rt_path_error("open", checked, errno);
        return false;
    }

    struct stat information;
    if (fstat(descriptor, &information) != 0) {
        int error = errno;
        close(descriptor);
        *message = pcy_rt_path_error("stat", checked, error);
        return false;
    }

    int64_t size = information.st_size < 0 ? 0 : (int64_t)information.st_size;
    if ((uint64_t)size > PCY_RT_MAX_FILE_SIZE) {
        close(descriptor);
        *message = pcy_rt_plain_error("file is larger than the maximum supported size");
        return false;
    }

    pcy_string *result = pcy_rt_string_allocate(size);
    char *bytes = pcy_rt_string_bytes(result);
    int64_t total = 0;
    while (total < size) {
        ssize_t count = read(descriptor, bytes + total, (size_t)(size - total));
        if (count < 0) {
            int error = errno;
            close(descriptor);
            *message = pcy_rt_path_error("read", checked, error);
            return false;
        }
        if (count == 0) {
            break;
        }
        total += (int64_t)count;
    }
    close(descriptor);
    result->length = total;
    bytes[total] = '\0';
    *text = result;
    return true;
}

/* Builds the C argv vector in the parent. */
static bool pcy_rt_build_argv(const pcy_string *program, const pcy_array *args, char ***argv_out, const pcy_string **message) {
    if (args == NULL) {
        pcy_rt_fail("try_run received a null argument array");
    }
    if (args->element_size != (int64_t)sizeof(const pcy_string *)) {
        pcy_rt_fail("try_run received a non-string argument array");
    }
    int64_t count = args->length;
    if ((uint64_t)count > SIZE_MAX / sizeof(char *) - 2) {
        *message = pcy_rt_path_error_with_reason("fork/exec", program, "argument list too long");
        return false;
    }

    char **argv = (char **)malloc(((size_t)count + 2) * sizeof(char *));
    if (argv == NULL) {
        *message = pcy_rt_path_error_with_reason("fork/exec", program, "cannot allocate argument vector");
        return false;
    }
    argv[0] = (char *)program->data;
    const pcy_string **items = (const pcy_string **)args->data;
    for (int64_t index = 0; index < count; index++) {
        const pcy_string *item = items[index];
        argv[index + 1] = item == NULL ? (char *)"" : (char *)item->data;
    }
    argv[count + 1] = NULL;
    *argv_out = argv;
    return true;
}

bool pcy_rt_try_run(const pcy_string *program, const pcy_array *args, int64_t *status, const pcy_string **message) {
    if (status == NULL || message == NULL) {
        pcy_rt_fail("try_run received a null result slot");
    }
    *status = 0;
    *message = NULL;

    const pcy_string *checked = pcy_rt_string_value(program);
    char **argv = NULL;
    if (!pcy_rt_build_argv(checked, args, &argv, message)) {
        return false;
    }

    int error_pipe[2];
    if (pipe(error_pipe) != 0) {
        int error = errno;
        free(argv);
        *message = pcy_rt_path_error("fork/exec", checked, error);
        return false;
    }
    if (fcntl(error_pipe[1], F_SETFD, FD_CLOEXEC) != 0) {
        int error = errno;
        close(error_pipe[0]);
        close(error_pipe[1]);
        free(argv);
        *message = pcy_rt_path_error("fork/exec", checked, error);
        return false;
    }

    pcy_rt_flush_streams();
    pid_t child = fork();
    if (child < 0) {
        int error = errno;
        close(error_pipe[0]);
        close(error_pipe[1]);
        free(argv);
        *message = pcy_rt_path_error("fork/exec", checked, error);
        return false;
    }
    if (child == 0) {
        close(error_pipe[0]);
        execve(checked->data, argv, environ);
        int child_error = errno;
        ssize_t ignored = write(error_pipe[1], &child_error, sizeof(child_error));
        (void)ignored;
        _exit(127);
    }

    close(error_pipe[1]);
    int child_error = 0;
    ssize_t received;
    int read_error = 0;
    do {
        received = read(error_pipe[0], &child_error, sizeof(child_error));
    } while (received < 0 && errno == EINTR);
    if (received < 0) {
        read_error = errno;
    }
    close(error_pipe[0]);

    int wait_status = 0;
    pid_t waited;
    do {
        waited = waitpid(child, &wait_status, 0);
    } while (waited < 0 && errno == EINTR);
    int wait_error = waited < 0 ? errno : 0;
    free(argv);

    if (waited < 0) {
        *message = pcy_rt_path_error("wait", checked, wait_error);
        return false;
    }
    if (received < 0) {
        *message = pcy_rt_path_error("fork/exec", checked, read_error);
        return false;
    }
    if (received > 0) {
        *message = pcy_rt_path_error("fork/exec", checked, child_error);
        return false;
    }
    if (WIFEXITED(wait_status)) {
        *status = (int64_t)WEXITSTATUS(wait_status);
        return true;
    }
    if (WIFSIGNALED(wait_status)) {
        *status = (int64_t)(128 + WTERMSIG(wait_status));
        return true;
    }
    *message = pcy_rt_plain_error("wait returned an unexpected status");
    return false;
}

#define PCY_RT_CAPTURE_LIMIT ((int64_t)16777216)

struct pcy_rt_capture_buffer {
    char *data;
    size_t length;
    size_t capacity;
};

static void pcy_rt_capture_free(struct pcy_rt_capture_buffer *buffer) {
    free(buffer->data);
    buffer->data = NULL;
    buffer->length = 0;
    buffer->capacity = 0;
}

/* Returns 1 on success, 0 when the capture limit would be exceeded, -1 on OOM. */
static int pcy_rt_capture_append(struct pcy_rt_capture_buffer *buffer, const char *bytes, size_t count) {
    if ((int64_t)count > PCY_RT_CAPTURE_LIMIT - (int64_t)buffer->length) {
        return 0;
    }
    size_t needed = buffer->length + count;
    if (needed > buffer->capacity) {
        size_t capacity = buffer->capacity == 0 ? 4096 : buffer->capacity;
        while (capacity < needed) {
            if (capacity > SIZE_MAX / 2) {
                return -1;
            }
            capacity *= 2;
        }
        char *grown = (char *)realloc(buffer->data, capacity);
        if (grown == NULL) {
            return -1;
        }
        buffer->data = grown;
        buffer->capacity = capacity;
    }
    if (count > 0) {
        memcpy(buffer->data + buffer->length, bytes, count);
        buffer->length = needed;
    }
    return 1;
}

static const pcy_string *pcy_rt_capture_string(struct pcy_rt_capture_buffer *buffer) {
    pcy_string *result = pcy_rt_string_allocate((int64_t)buffer->length);
    if (buffer->length > 0) {
        memcpy(pcy_rt_string_bytes(result), buffer->data, buffer->length);
    }
    return result;
}

static const pcy_string *pcy_rt_capture_message(const pcy_string *program, const char *stream, const char *suffix) {
    char reason[160];
    snprintf(reason, sizeof(reason), "%s %s", stream, suffix);
    return pcy_rt_path_error_with_reason("run", program, reason);
}

static const pcy_string *pcy_rt_capture_nul_message(const pcy_string *program, const char *stream, int64_t offset) {
    char reason[160];
    snprintf(reason, sizeof(reason), "%s contains a NUL byte at offset %lld", stream, (long long)offset);
    return pcy_rt_path_error_with_reason("run", program, reason);
}

static bool pcy_rt_make_capture_pipe(int descriptors[2]) {
    if (pipe(descriptors) != 0) {
        return false;
    }
    if (fcntl(descriptors[0], F_SETFD, FD_CLOEXEC) != 0 ||
        fcntl(descriptors[1], F_SETFD, FD_CLOEXEC) != 0) {
        int error = errno;
        close(descriptors[0]);
        close(descriptors[1]);
        errno = error;
        return false;
    }
    return true;
}

static pid_t pcy_rt_wait_capture(pid_t child, int *wait_status, int *wait_error) {
    pid_t waited;
    do {
        waited = waitpid(child, wait_status, 0);
    } while (waited < 0 && errno == EINTR);
    if (waited < 0) {
        *wait_error = errno;
    }
    return waited;
}

bool pcy_rt_try_run_capture(const pcy_string *program, const pcy_array *args,
                            int64_t *status, const pcy_string **out,
                            const pcy_string **err, const pcy_string **message) {
    if (status == NULL || out == NULL || err == NULL || message == NULL) {
        pcy_rt_fail("try_run_capture received a null result slot");
    }
    *status = 0;
    *out = NULL;
    *err = NULL;
    *message = NULL;

    const pcy_string *checked = pcy_rt_string_value(program);
    char **argv = NULL;
    if (!pcy_rt_build_argv(checked, args, &argv, message)) {
        return false;
    }

    int null_descriptor = open("/dev/null", O_RDONLY | O_CLOEXEC);
    if (null_descriptor < 0) {
        int error = errno;
        free(argv);
        *message = pcy_rt_path_error("run", checked, error);
        return false;
    }

    int out_pipe[2];
    int err_pipe[2];
    int error_pipe[2];
    if (!pcy_rt_make_capture_pipe(out_pipe)) {
        int error = errno;
        close(null_descriptor);
        free(argv);
        *message = pcy_rt_path_error("run", checked, error);
        return false;
    }
    if (!pcy_rt_make_capture_pipe(err_pipe)) {
        int error = errno;
        close(null_descriptor);
        close(out_pipe[0]);
        close(out_pipe[1]);
        free(argv);
        *message = pcy_rt_path_error("run", checked, error);
        return false;
    }
    if (!pcy_rt_make_capture_pipe(error_pipe)) {
        int error = errno;
        close(null_descriptor);
        close(out_pipe[0]);
        close(out_pipe[1]);
        close(err_pipe[0]);
        close(err_pipe[1]);
        free(argv);
        *message = pcy_rt_path_error("run", checked, error);
        return false;
    }

    pcy_rt_flush_streams();
    pid_t child = fork();
    if (child < 0) {
        int error = errno;
        close(null_descriptor);
        close(out_pipe[0]);
        close(out_pipe[1]);
        close(err_pipe[0]);
        close(err_pipe[1]);
        close(error_pipe[0]);
        close(error_pipe[1]);
        free(argv);
        *message = pcy_rt_path_error("fork/exec", checked, error);
        return false;
    }
    if (child == 0) {
        close(out_pipe[0]);
        close(err_pipe[0]);
        close(error_pipe[0]);
        if (dup2(null_descriptor, STDIN_FILENO) < 0 ||
            dup2(out_pipe[1], STDOUT_FILENO) < 0 ||
            dup2(err_pipe[1], STDERR_FILENO) < 0) {
            int child_error = errno;
            ssize_t ignored = write(error_pipe[1], &child_error, sizeof(child_error));
            (void)ignored;
            _exit(127);
        }
        close(null_descriptor);
        close(out_pipe[1]);
        close(err_pipe[1]);
        execve(checked->data, argv, environ);
        int child_error = errno;
        ssize_t ignored = write(error_pipe[1], &child_error, sizeof(child_error));
        (void)ignored;
        _exit(127);
    }

    close(null_descriptor);
    close(out_pipe[1]);
    close(err_pipe[1]);
    close(error_pipe[1]);

    struct pcy_rt_capture_buffer captured_out = { NULL, 0, 0 };
    struct pcy_rt_capture_buffer captured_err = { NULL, 0, 0 };
    int child_error = 0;
    bool child_error_received = false;
    bool out_open = true;
    bool err_open = true;
    bool error_open = true;
    bool failed = false;
    const pcy_string *failure_message = NULL;

    while (out_open || err_open || error_open) {
        struct pollfd poll_descriptors[3];
        int indexes[3];
        int count = 0;
        if (out_open) {
            poll_descriptors[count].fd = out_pipe[0];
            poll_descriptors[count].events = POLLIN;
            poll_descriptors[count].revents = 0;
            indexes[count] = 0;
            count++;
        }
        if (err_open) {
            poll_descriptors[count].fd = err_pipe[0];
            poll_descriptors[count].events = POLLIN;
            poll_descriptors[count].revents = 0;
            indexes[count] = 1;
            count++;
        }
        if (error_open) {
            poll_descriptors[count].fd = error_pipe[0];
            poll_descriptors[count].events = POLLIN;
            poll_descriptors[count].revents = 0;
            indexes[count] = 2;
            count++;
        }
        int ready;
        do {
            ready = poll(poll_descriptors, (nfds_t)count, -1);
        } while (ready < 0 && errno == EINTR);
        if (ready < 0) {
            failure_message = pcy_rt_path_error("run", checked, errno);
            failed = true;
            break;
        }
        for (int index = 0; index < count; index++) {
            if (poll_descriptors[index].revents == 0) {
                continue;
            }
            int stream = indexes[index];
            int descriptor = poll_descriptors[index].fd;
            if (stream == 2) {
                ssize_t received;
                do {
                    received = read(descriptor, &child_error, sizeof(child_error));
                } while (received < 0 && errno == EINTR);
                if (received > 0) {
                    child_error_received = true;
                }
                if (received <= 0) {
                    close(descriptor);
                    error_open = false;
                }
                continue;
            }
            char bytes[8192];
            ssize_t received;
            do {
                received = read(descriptor, bytes, sizeof(bytes));
            } while (received < 0 && errno == EINTR);
            if (received > 0) {
                int appended = pcy_rt_capture_append(stream == 0 ? &captured_out : &captured_err,
                                                     bytes, (size_t)received);
                if (appended == 0) {
                    failure_message = pcy_rt_capture_message(checked, stream == 0 ? "stdout" : "stderr",
                                                              "exceeds the 16777216 byte limit");
                    failed = true;
                    break;
                }
                if (appended < 0) {
                    failure_message = pcy_rt_plain_error("capture allocation failed");
                    failed = true;
                    break;
                }
            } else if (received == 0) {
                close(descriptor);
                if (stream == 0) {
                    out_open = false;
                } else {
                    err_open = false;
                }
            } else {
                failure_message = pcy_rt_path_error("run", checked, errno);
                failed = true;
                break;
            }
        }
        if (failed) {
            break;
        }
    }

    if (failed) {
        kill(child, SIGKILL);
    }
    if (out_open) {
        close(out_pipe[0]);
    }
    if (err_open) {
        close(err_pipe[0]);
    }
    if (error_open) {
        close(error_pipe[0]);
    }

    int wait_status = 0;
    int wait_error = 0;
    pid_t waited = pcy_rt_wait_capture(child, &wait_status, &wait_error);
    free(argv);

    if (failed) {
        pcy_rt_capture_free(&captured_out);
        pcy_rt_capture_free(&captured_err);
        *message = failure_message;
        return false;
    }
    if (waited < 0) {
        pcy_rt_capture_free(&captured_out);
        pcy_rt_capture_free(&captured_err);
        *message = pcy_rt_path_error("wait", checked, wait_error);
        return false;
    }
    if (child_error_received) {
        pcy_rt_capture_free(&captured_out);
        pcy_rt_capture_free(&captured_err);
        *message = pcy_rt_path_error("fork/exec", checked, child_error);
        return false;
    }

    if (!WIFEXITED(wait_status) && !WIFSIGNALED(wait_status)) {
        pcy_rt_capture_free(&captured_out);
        pcy_rt_capture_free(&captured_err);
        *message = pcy_rt_plain_error("wait returned an unexpected status");
        return false;
    }
    if (WIFEXITED(wait_status)) {
        *status = (int64_t)WEXITSTATUS(wait_status);
    } else {
        *status = (int64_t)(128 + WTERMSIG(wait_status));
    }

    for (size_t index = 0; index < captured_out.length; index++) {
        if (captured_out.data[index] == '\0') {
            pcy_rt_capture_free(&captured_out);
            pcy_rt_capture_free(&captured_err);
            *message = pcy_rt_capture_nul_message(checked, "stdout", (int64_t)index);
            return false;
        }
    }
    for (size_t index = 0; index < captured_err.length; index++) {
        if (captured_err.data[index] == '\0') {
            pcy_rt_capture_free(&captured_out);
            pcy_rt_capture_free(&captured_err);
            *message = pcy_rt_capture_nul_message(checked, "stderr", (int64_t)index);
            return false;
        }
    }

    *out = pcy_rt_capture_string(&captured_out);
    *err = pcy_rt_capture_string(&captured_err);
    pcy_rt_capture_free(&captured_out);
    pcy_rt_capture_free(&captured_err);
    return true;
}

int pcy_rt_try_file_info(const pcy_string *path, bool *is_file, bool *is_directory,
                         bool *is_symlink, bool *executable, int64_t *size,
                         const pcy_string **message) {
    if (is_file == NULL || is_directory == NULL || is_symlink == NULL ||
        executable == NULL || size == NULL || message == NULL) {
        pcy_rt_fail("try_file_info received a null result slot");
    }
    *is_file = false;
    *is_directory = false;
    *is_symlink = false;
    *executable = false;
    *size = 0;
    *message = NULL;

    const pcy_string *checked = pcy_rt_string_value(path);
    struct stat link_info;
    if (lstat(checked->data, &link_info) != 0) {
        int error = errno;
        if (error == ENOENT || error == ENOTDIR) {
            return PCY_RT_FILE_INFO_MISSING;
        }
        *message = pcy_rt_path_error("lstat", checked, error);
        return PCY_RT_FILE_INFO_FAILED;
    }

    bool link = S_ISLNK(link_info.st_mode);
    *is_symlink = link;
    struct stat followed = link_info;
    if (link) {
        if (stat(checked->data, &followed) != 0) {
            int error = errno;
            if (error == ENOENT || error == ENOTDIR) {
                return PCY_RT_FILE_INFO_OK;
            }
            *message = pcy_rt_path_error("stat", checked, error);
            return PCY_RT_FILE_INFO_FAILED;
        }
    }

    *is_file = S_ISREG(followed.st_mode);
    *is_directory = S_ISDIR(followed.st_mode);
    if (*is_file) {
        *size = (int64_t)followed.st_size;
    }
    *executable = access(checked->data, X_OK) == 0;
    return PCY_RT_FILE_INFO_OK;
}

bool pcy_rt_try_getenv(const pcy_string *name, const pcy_string **value) {
    if (value == NULL) {
        pcy_rt_fail("try_getenv received a null result slot");
    }
    *value = NULL;

    const pcy_string *checked = pcy_rt_string_value(name);
    if (checked->length == 0 || strchr(checked->data, '=') != NULL) {
        return false;
    }
    const char *found = getenv(checked->data);
    if (found == NULL) {
        return false;
    }
    size_t length = strlen(found);
    pcy_string *result = pcy_rt_string_allocate((int64_t)length);
    memcpy(pcy_rt_string_bytes(result), found, length);
    *value = result;
    return true;
}

/* Returns getcwd's absolute path as a fresh GC-managed string. */
static pcy_string *pcy_rt_getcwd_string(const pcy_string **message) {
    size_t capacity = 256;
    char *buffer = (char *)malloc(capacity);
    if (buffer == NULL) {
        *message = pcy_rt_operation_error("getwd", ENOMEM);
        return NULL;
    }
    for (;;) {
        if (getcwd(buffer, capacity) != NULL) {
            size_t length = strlen(buffer);
            pcy_string *result = pcy_rt_string_allocate((int64_t)length);
            memcpy(pcy_rt_string_bytes(result), buffer, length);
            free(buffer);
            return result;
        }
        int error = errno;
        if (error != ERANGE) {
            free(buffer);
            *message = pcy_rt_operation_error("getwd", error);
            return NULL;
        }
        if (capacity > SIZE_MAX / 2) {
            free(buffer);
            *message = pcy_rt_operation_error("getwd", ENAMETOOLONG);
            return NULL;
        }
        size_t next = capacity * 2;
        char *grown = (char *)realloc(buffer, next);
        if (grown == NULL) {
            free(buffer);
            *message = pcy_rt_operation_error("getwd", ENOMEM);
            return NULL;
        }
        buffer = grown;
        capacity = next;
    }
}

bool pcy_rt_try_current_directory(const pcy_string **path, const pcy_string **message) {
    if (path == NULL || message == NULL) {
        pcy_rt_fail("try_current_directory received a null result slot");
    }
    *path = NULL;
    *message = NULL;
    *path = pcy_rt_getcwd_string(message);
    return *path != NULL;
}

/* Reports whether a temporary-directory prefix is a safe literal component. */
static bool pcy_rt_valid_temp_prefix(const pcy_string *prefix) {
    if (prefix->length <= 0 || prefix->length > 64) {
        return false;
    }
    if (prefix->length == 2 && prefix->data[0] == '.' && prefix->data[1] == '.') {
        return false;
    }
    for (int64_t index = 0; index < prefix->length; index++) {
        unsigned char value = (unsigned char)prefix->data[index];
        bool allowed = (value >= 'A' && value <= 'Z') ||
                       (value >= 'a' && value <= 'z') ||
                       (value >= '0' && value <= '9') ||
                       value == '.' || value == '_' || value == '-';
        if (!allowed) {
            return false;
        }
    }
    return true;
}

/* Copies a byte range into a fresh GC-managed string. */
static pcy_string *pcy_rt_string_copy_bytes(const char *bytes, size_t length) {
    pcy_string *result = pcy_rt_string_allocate((int64_t)length);
    memcpy(pcy_rt_string_bytes(result), bytes, length);
    return result;
}

/* Joins two path pieces with exactly one separator between them. */
static pcy_string *pcy_rt_string_join_path(const pcy_string *left, const pcy_string *right) {
    bool left_has_separator = left->length > 0 && left->data[left->length - 1] == '/';
    size_t length = (size_t)left->length + (left_has_separator ? 0 : 1) + (size_t)right->length;
    pcy_string *result = pcy_rt_string_allocate((int64_t)length);
    char *bytes = pcy_rt_string_bytes(result);
    memcpy(bytes, left->data, (size_t)left->length);
    size_t offset = (size_t)left->length;
    if (!left_has_separator) {
        bytes[offset] = '/';
        offset++;
    }
    memcpy(bytes + offset, right->data, (size_t)right->length);
    return result;
}

bool pcy_rt_try_create_temp_directory(const pcy_string *prefix,
                                      const pcy_string **path,
                                      const pcy_string **message) {
    if (path == NULL || message == NULL) {
        pcy_rt_fail("try_create_temp_directory received a null result slot");
    }
    *path = NULL;
    *message = NULL;

    const pcy_string *checked = pcy_rt_string_value(prefix);
    if (!pcy_rt_valid_temp_prefix(checked)) {
        *message = pcy_rt_plain_error("invalid temporary directory prefix");
        return false;
    }

    const char *temporary = getenv("TMPDIR");
    pcy_string *base = NULL;
    if (temporary == NULL || temporary[0] == '\0') {
        base = pcy_rt_string_copy_bytes("/tmp", 4);
    } else {
        size_t temporary_length = strlen(temporary);
        pcy_string *configured = pcy_rt_string_copy_bytes(temporary, temporary_length);
        if (configured->data[0] == '/') {
            base = configured;
        } else {
            pcy_string *current = pcy_rt_getcwd_string(message);
            if (current == NULL) {
                return false;
            }
            base = pcy_rt_string_join_path(current, configured);
        }
    }

    pcy_string *suffix = pcy_rt_string_copy_bytes(".XXXXXX", 7);
    const pcy_string *template_text = pcy_rt_string_join_path(base, checked);
    template_text = pcy_rt_string_concat(template_text, suffix);

    size_t template_length = (size_t)template_text->length;
    char *template = (char *)malloc(template_length + 1);
    if (template == NULL) {
        pcy_rt_fail("try_create_temp_directory could not allocate a path template");
    }
    memcpy(template, template_text->data, template_length + 1);

    /*
     * mkdtemp requests mode 0700, but the process umask still applies. This
     * runtime is not required to be thread-safe, so clear the umask only around
     * the atomic creation instead of racing a later chmod with another process
     * that can rename or replace the new directory.
     */
    mode_t previous_umask = umask(0);
    char *created = mkdtemp(template);
    int creation_error = errno;
    umask(previous_umask);
    errno = creation_error;
    if (created == NULL) {
        int error = creation_error;
        if (error == ENOENT || error == ENOTDIR) {
            struct stat base_info;
            if (stat(base->data, &base_info) != 0) {
                int check = errno;
                if (check == ENOENT || check == ENOTDIR) {
                    free(template);
                    *message = pcy_rt_path_error("stat", base, check);
                    return false;
                }
            }
        }
        free(template);
        *message = pcy_rt_path_error("mkdir", template_text, error);
        return false;
    }

    size_t created_length = strlen(created);
    *path = pcy_rt_string_copy_bytes(created, created_length);
    free(template);
    return true;
}

/* Writes every byte or reports the first write failure. */
static bool pcy_rt_write_all(int descriptor, const char *bytes, int64_t length, const pcy_string *path, const pcy_string **message) {
    int64_t total = 0;
    while (total < length) {
        ssize_t count = write(descriptor, bytes + total, (size_t)(length - total));
        if (count < 0) {
            if (errno == EINTR) {
                continue;
            }
            *message = pcy_rt_path_error("write", path, errno);
            return false;
        }
        if (count == 0) {
            *message = pcy_rt_path_error_with_reason("write", path, "short write");
            return false;
        }
        total += (int64_t)count;
    }
    return true;
}

/* Creates or truncates path and writes length bytes with mode 0644. */
static bool pcy_rt_try_write_common(const pcy_string *path, const char *bytes, int64_t length, const pcy_string **message) {
    if (message == NULL) {
        pcy_rt_fail("try_write received a null result slot");
    }
    *message = NULL;

    const pcy_string *checked = pcy_rt_string_value(path);
    int descriptor = open(checked->data, O_WRONLY | O_CREAT | O_TRUNC, 0644);
    if (descriptor < 0) {
        *message = pcy_rt_path_error("open", checked, errno);
        return false;
    }

    if (!pcy_rt_write_all(descriptor, bytes, length, checked, message)) {
        close(descriptor);
        return false;
    }
    if (close(descriptor) != 0) {
        *message = pcy_rt_path_error("close", checked, errno);
        return false;
    }
    return true;
}

bool pcy_rt_try_write_file(const pcy_string *path, const pcy_string *text, const pcy_string **message) {
    const pcy_string *checked = pcy_rt_string_value(text);
    return pcy_rt_try_write_common(path, checked->data, checked->length, message);
}

bool pcy_rt_try_write_bytes(const pcy_string *path, const pcy_array *data, const pcy_string **message) {
    if (data == NULL) {
        pcy_rt_fail("try_write_bytes received a null byte array");
    }
    if (data->element_size != (int64_t)sizeof(uint8_t)) {
        pcy_rt_fail("try_write_bytes received a non-byte array");
    }
    return pcy_rt_try_write_common(path, (const char *)data->data, data->length, message);
}

bool pcy_rt_try_create_directory(const pcy_string *path, const pcy_string **message) {
    if (message == NULL) {
        pcy_rt_fail("try_create_directory received a null result slot");
    }
    *message = NULL;
    const pcy_string *checked = pcy_rt_string_value(path);
    if (mkdir(checked->data, 0755) != 0) {
        *message = pcy_rt_path_error("mkdir", checked, errno);
        return false;
    }
    return true;
}

bool pcy_rt_try_remove(const pcy_string *path, const pcy_string **message) {
    if (message == NULL) {
        pcy_rt_fail("try_remove received a null result slot");
    }
    *message = NULL;
    const pcy_string *checked = pcy_rt_string_value(path);

    struct stat information;
    if (lstat(checked->data, &information) != 0) {
        *message = pcy_rt_path_error("remove", checked, errno);
        return false;
    }
    int result = S_ISDIR(information.st_mode) ? rmdir(checked->data) : unlink(checked->data);
    if (result != 0) {
        *message = pcy_rt_path_error("remove", checked, errno);
        return false;
    }
    return true;
}

/* Orders two pcy_string pointers by their NUL-terminated name bytes. */
static int pcy_rt_name_compare(const void *left, const void *right) {
    const pcy_string *a = *(const pcy_string *const *)left;
    const pcy_string *b = *(const pcy_string *const *)right;
    return strcmp(a->data, b->data);
}

bool pcy_rt_try_list_directory(const pcy_string *path, pcy_array **entries, const pcy_string **message) {
    if (entries == NULL || message == NULL) {
        pcy_rt_fail("try_list_directory received a null result slot");
    }
    *entries = NULL;
    *message = NULL;

    const pcy_string *checked = pcy_rt_string_value(path);
    DIR *directory = opendir(checked->data);
    if (directory == NULL) {
        *message = pcy_rt_path_error("open", checked, errno);
        return false;
    }

    int64_t count = 0;
    errno = 0;
    struct dirent *entry;
    while ((entry = readdir(directory)) != NULL) {
        if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0) {
            continue;
        }
        count++;
    }
    if (errno != 0) {
        int error = errno;
        closedir(directory);
        *message = pcy_rt_path_error("readdirent", checked, error);
        return false;
    }

    pcy_array *result = pcy_rt_array_new(count, (int64_t)sizeof(const pcy_string *));
    rewinddir(directory);
    int64_t index = 0;
    errno = 0;
    while ((entry = readdir(directory)) != NULL) {
        if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0) {
            continue;
        }
        size_t name_length = strlen(entry->d_name);
        pcy_string *name = pcy_rt_string_allocate((int64_t)name_length);
        memcpy(pcy_rt_string_bytes(name), entry->d_name, name_length);
        ((const pcy_string **)result->data)[index] = name;
        index++;
    }
    if (errno != 0) {
        int error = errno;
        closedir(directory);
        *message = pcy_rt_path_error("readdirent", checked, error);
        return false;
    }
    closedir(directory);

    qsort(result->data, (size_t)index, sizeof(const pcy_string *), pcy_rt_name_compare);
    result->length = index;
    *entries = result;
    return true;
}

const pcy_string *pcy_rt_read_file(const pcy_string *path) {
    const pcy_string *checked = pcy_rt_string_value(path);

    FILE *file = fopen(checked->data, "rb");
    if (file == NULL) {
        int error = errno;
        pcy_rt_fail_path("read_file: cannot open", checked, error);
    }

    if (fseek(file, 0, SEEK_END) != 0) {
        int error = errno;
        fclose(file);
        pcy_rt_fail_path("read_file: cannot determine the size of", checked, error);
    }

    long length = ftell(file);
    if (length < 0) {
        int error = errno;
        fclose(file);
        pcy_rt_fail_path("read_file: cannot determine the size of", checked, error);
    }
    if ((unsigned long)length > PCY_RT_MAX_FILE_SIZE) {
        fclose(file);
        pcy_rt_fail("read_file: file is larger than the maximum supported size");
    }

    if (fseek(file, 0, SEEK_SET) != 0) {
        int error = errno;
        fclose(file);
        pcy_rt_fail_path("read_file: cannot rewind", checked, error);
    }

    pcy_string *result = pcy_rt_string_allocate((int64_t)length);
    size_t read = fread(pcy_rt_string_bytes(result), 1, (size_t)length, file);
    if (ferror(file)) {
        int error = errno;
        fclose(file);
        pcy_rt_fail_path("read_file: cannot read", checked, error);
    }

    /* A file that shrank while it was being read yields the bytes actually read,
     * so the stored length is adjusted to match the bytes present. */
    result->length = (int64_t)read;
    pcy_rt_string_bytes(result)[read] = '\0';
    fclose(file);
    return result;
}

void pcy_rt_print_int(int64_t value) {
    printf("%lld", (long long)value);
}

void pcy_rt_print_float(double value) {
    printf("%g", value);
}

void pcy_rt_print_bool(bool value) {
    fputs(value ? "true" : "false", stdout);
}

void pcy_rt_print_string(const pcy_string *value) {
    const pcy_string *checked = pcy_rt_string_value(value);
    if (checked->length > 0) {
        fwrite(checked->data, 1, (size_t)checked->length, stdout);
    }
}

void pcy_rt_print_space(void) {
    fputc(' ', stdout);
}

void pcy_rt_print_newline(void) {
    fputc('\n', stdout);
}

void pcy_rt_eprint_int(int64_t value) {
    fprintf(stderr, "%lld", (long long)value);
}

void pcy_rt_eprint_float(double value) {
    fprintf(stderr, "%g", value);
}

void pcy_rt_eprint_bool(bool value) {
    fputs(value ? "true" : "false", stderr);
}

void pcy_rt_eprint_string(const pcy_string *value) {
    const pcy_string *checked = pcy_rt_string_value(value);
    if (checked->length > 0) {
        fwrite(checked->data, 1, (size_t)checked->length, stderr);
    }
}

void pcy_rt_eprint_space(void) {
    fputc(' ', stderr);
}

void pcy_rt_eprint_newline(void) {
    /* Keep output written by earlier print calls ahead of this diagnostic. */
    fflush(stdout);
    fputc('\n', stderr);
    pcy_rt_flush_streams();
}
