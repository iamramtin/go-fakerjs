/*
 * Module harness: QuickJS-NG + the Faker.js bundle as a WASI reactor.
 *
 * See wasm/README.md for the rationale behind each decision. Strings cross the boundary as
 * (ptr, len) pairs in linear memory; results come back as one malloc'd, length-prefixed buffer that the
 * host releases with free(). Nothing the host sends is ever evaluated as JavaScript source.
 */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/time.h>
#include <time.h>

/* Amalgamated so the harness can reach JSContext::random_state (see reseed_math_random). */
#include "quickjs.c"

#ifndef BUNDLE_FILE
#error "define BUNDLE_FILE to the path of the faker bundle (-DBUNDLE_FILE='\"...\"')"
#endif

#define EXPORT(name) __attribute__((export_name(name)))

/* Tunables baked into the snapshot. */
#define WASM_STACK_BYTES (2u << 20) /* must match -Wl,-z,stack-size in build.sh */
#define JS_STACK_BYTES (1u << 20)   /* JS_SetMaxStackSize: half the wasm stack leaves room for native frames */
#define JS_MEMORY_LIMIT ((size_t)160 << 20)
#define MAX_RESULT_BYTES ((size_t)64 << 20)
#define MAX_PATH_BYTES 256

/* Status codes in the first byte of a call() result buffer. Keep in sync with internal/engine. */
enum {
    ST_OK = 0,
    ST_UNKNOWN_METHOD = 1,
    ST_JS_EXCEPTION = 2,
    ST_INVALID_ARG = 3,
    ST_INTERRUPTED = 4,
    ST_OUT_OF_MEMORY = 5,
    ST_RESULT_TOO_LARGE = 6,
    ST_INTERNAL = 7,
    ST_INVALID_METHOD = 8,
};

/* Writable on purpose: wizer-initialize zeroes it after evaluation so the snapshot does not carry the source. */
static unsigned char bundle_data[] = {
#embed BUNDLE_FILE
    , 0};

static JSRuntime *rt;
static JSContext *ctx;
static JSValue faker = JS_UNDEFINED;

static char **allow; /* sorted "module.method" names, built once at init */
static size_t allow_len;

static int64_t deadline_ns;  /* 0 = none */
static int interrupted;      /* set by the interrupt handler when it fires */
static int in_init;          /* 1 only inside wizer-initialize: clocks return a fixed value */
static char last_err[512];

#ifdef GC_STRESS
/* scripts/gcstress.sh builds with a patched quickjs.c whose force_gc is this flag (QuickJS's own
 * FORCE_GC_AT_MALLOC, but switchable so Wizer's initialisation does not run a full GC per allocation). */
int g_gc_stress;
EXPORT("set_gc_stress")
void set_gc_stress(int on)
{
    g_gc_stress = on;
}
#endif

/* ---- host imports ---- */

__attribute__((import_module("fakerjs"), import_name("monotonic_ns"))) extern uint64_t host_monotonic_ns(void);

/* ---- deterministic clock during Wizer pre-initialisation ---- */

extern int __real_gettimeofday(struct timeval *tv, void *tz);
extern int __real_clock_gettime(clockid_t id, struct timespec *ts);

int __wrap_gettimeofday(struct timeval *tv, void *tz)
{
    if (in_init) {
        tv->tv_sec = 946684800; /* 2000-01-01T00:00:00Z */
        tv->tv_usec = 0;
        return 0;
    }
    return __real_gettimeofday(tv, tz);
}

int __wrap_clock_gettime(clockid_t id, struct timespec *ts)
{
    if (in_init) {
        ts->tv_sec = 946684800;
        ts->tv_nsec = 0;
        return 0;
    }
    return __real_clock_gettime(id, ts);
}

/* ---- helpers ---- */

static void set_err(const char *msg)
{
    snprintf(last_err, sizeof last_err, "%s", msg);
}

typedef struct {
    uint8_t *p;
    size_t len, cap;
} Buf;

static int buf_put(Buf *b, const void *data, size_t n)
{
    if (b->len + n > MAX_RESULT_BYTES)
        return -1;
    if (b->len + n > b->cap) {
        size_t cap = b->cap ? b->cap : 256;
        while (cap < b->len + n)
            cap *= 2;
        uint8_t *np = realloc(b->p, cap);
        if (!np)
            return -2;
        b->p = np;
        b->cap = cap;
    }
    memcpy(b->p + b->len, data, n);
    b->len += n;
    return 0;
}

static int buf_u32(Buf *b, uint32_t v)
{
    return buf_put(b, &v, 4); /* wasm is little-endian */
}

/* Returns a result buffer: u32 payload_len, u8 status, payload. Consumes b. */
static uint32_t finish(Buf *b, uint8_t status, const char *msg)
{
    if (status != ST_OK) {
        free(b->p);
        b->p = NULL;
        b->len = b->cap = 0;
    }
    size_t plen = status == ST_OK ? b->len : strlen(msg);
    uint8_t *out = malloc(5 + plen);
    if (!out) {
        free(b->p);
        return 0;
    }
    uint32_t l = (uint32_t)plen;
    memcpy(out, &l, 4);
    out[4] = status;
    if (status == ST_OK)
        memcpy(out + 5, b->p, plen);
    else
        memcpy(out + 5, msg, plen);
    free(b->p);
    return (uint32_t)(uintptr_t)out;
}

/* Describes the pending exception into msg and classifies it. */
static int take_exception(char *msg, size_t msglen)
{
    JSValue ex = JS_GetException(ctx);
    const char *s = JS_ToCString(ctx, ex);
    snprintf(msg, msglen, "%s", s ? s : "exception");
    if (s)
        JS_FreeCString(ctx, s);
    JS_FreeValue(ctx, ex);
    if (JS_HasException(ctx)) {
        JSValue ex2 = JS_GetException(ctx);
        JS_FreeValue(ctx, ex2);
    }
    if (interrupted)
        return ST_INTERRUPTED;
    if (strstr(msg, "out of memory"))
        return ST_OUT_OF_MEMORY;
    return ST_JS_EXCEPTION;
}

static int interrupt_cb(JSRuntime *r, void *opaque)
{
    (void)r;
    (void)opaque;
    if (deadline_ns == 0)
        return 0;
    if ((int64_t)host_monotonic_ns() >= deadline_ns) {
        interrupted = 1;
        return 1;
    }
    return 0;
}

static int cmp_names(const void *a, const void *b)
{
    return strcmp(*(char *const *)a, *(char *const *)b);
}

/* Mirrors the grammar ^[a-zA-Z][a-zA-Z0-9]*\.[a-zA-Z][a-zA-Z0-9]*$ . */
static int valid_path(const char *s, size_t n)
{
    size_t i = 0, dot = 0;
    for (int part = 0; part < 2; part++) {
        if (i >= n || !((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')))
            return 0;
        i++;
        while (i < n && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= '0' && s[i] <= '9')))
            i++;
        if (part == 0) {
            if (i >= n || s[i] != '.')
                return 0;
            dot = i++;
        }
    }
    return i == n && dot > 0;
}

/* ---- init-time JavaScript. Static source only; never built from host input. ---- */

/* Enumerates faker's module objects and their function-valued methods (own + prototype chain). */
static const char ALLOWLIST_JS[] =
    "(function () {\n"
    "  var skip = Object.getOwnPropertyNames(Object.prototype), names = [];\n"
    "  Reflect.ownKeys(faker).forEach(function (k) {\n"
    "    var mod = faker[k];\n"
    "    if (typeof k !== 'string' || k.charAt(0) === '_' || !mod || typeof mod !== 'object' || mod.faker !== faker) return;\n"
    "    var seen = Object.create(null), o = mod;\n"
    "    while (o && o !== Object.prototype) {\n"
    "      Reflect.ownKeys(o).forEach(function (n) {\n"
    "        if (typeof n !== 'string' || n.charAt(0) === '_' || n === 'constructor' || skip.indexOf(n) >= 0) return;\n"
    "        var d = Object.getOwnPropertyDescriptor(o, n);\n"
    "        if (d && typeof d.value === 'function' && !seen[n]) { seen[n] = true; names.push(k + '.' + n); }\n"
    "      });\n"
    "      o = Object.getPrototypeOf(o);\n"
    "    }\n"
    "  });\n"
    "  return names;\n"
    "})()";

/*
 * V8 appends the time zone display name to Date#toString / #toTimeString; QuickJS does not. This module has no
 * time zone database and always runs in UTC, so the V8 output in TZ=UTC is reproduced exactly.
 */
static const char DATE_SHIM_JS[] =
    "(function () {\n"
    "  var P = Date.prototype, SUFFIX = ' (Coordinated Universal Time)';\n"
    "  ['toString', 'toTimeString'].forEach(function (name) {\n"
    "    var orig = P[name];\n"
    "    var shim = { [name]: function () {\n"
    "      var s = orig.call(this);\n"
    "      return s === 'Invalid Date' ? s : s + SUFFIX;\n"
    "    } }[name];\n"
    "    Object.defineProperty(P, name, { value: shim, writable: true, enumerable: false, configurable: true });\n"
    "  });\n"
    "})()";

/* Replaces lone surrogates with U+FFFD so every result is valid UTF-8. */
static const char WELLFORMED_JS[] = "(function (s) { return s.toWellFormed(); })";

static JSValue well_formed_fn = JS_UNDEFINED;

static int eval_static(const char *src, size_t len, const char *name, JSValue *out)
{
    JSValue v = JS_Eval(ctx, src, len, name, JS_EVAL_TYPE_GLOBAL);
    if (JS_IsException(v)) {
        char msg[400];
        take_exception(msg, sizeof msg);
        snprintf(last_err, sizeof last_err, "init %s: %s", name, msg);
        return -1;
    }
    if (out)
        *out = v;
    else
        JS_FreeValue(ctx, v);
    return 0;
}

/* Evaluates the bundle through a stripped bytecode round trip so the retained heap carries neither the
 * 400 KB-2 MB source text nor debug info. */
static int load_bundle(unsigned char *src, size_t len)
{
    JSValue fn = JS_Eval(ctx, (const char *)src, len, "faker.js", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(fn)) {
        char msg[400];
        take_exception(msg, sizeof msg);
        snprintf(last_err, sizeof last_err, "compile bundle: %s", msg);
        return -1;
    }
    size_t blen;
    uint8_t *bc = JS_WriteObject(ctx, &blen, fn, JS_WRITE_OBJ_BYTECODE | JS_WRITE_OBJ_STRIP_SOURCE | JS_WRITE_OBJ_STRIP_DEBUG);
    JS_FreeValue(ctx, fn);
    if (!bc) {
        set_err("serialise bundle failed");
        return -1;
    }
    JSValue fn2 = JS_ReadObject(ctx, bc, blen, JS_READ_OBJ_BYTECODE);
    js_free(ctx, bc);
    if (JS_IsException(fn2)) {
        set_err("reload bundle failed");
        return -1;
    }
    JSValue r = JS_EvalFunction(ctx, fn2);
    if (JS_IsException(r)) {
        char msg[400];
        take_exception(msg, sizeof msg);
        snprintf(last_err, sizeof last_err, "run bundle: %s", msg);
        return -1;
    }
    JS_FreeValue(ctx, r);
    return 0;
}

/* ---- exports ---- */

/*
 * Snapshot point. Wizer runs this once at build time; everything it allocates is baked into the module.
 * No host import may be called here (Wizer forbids it), which is why clocks are fixed via in_init.
 */
EXPORT("wizer-initialize")
void wizer_initialize(void)
{
    in_init = 1;

    rt = JS_NewRuntime();
    if (!rt)
        abort();
    JS_SetMaxStackSize(rt, JS_STACK_BYTES);
    ctx = JS_NewContext(rt);
    if (!ctx)
        abort();
    /* Constant Math.random state so Faker's default random seed (drawn while the bundle loads) and the
     * snapshot are reproducible. Runtime entropy is mixed in by reseed_math_random. */
    ctx->random_state = 0x9e3779b97f4a7c15ull;

    int rc = load_bundle(bundle_data, sizeof bundle_data - 1);
    memset(bundle_data, 0, sizeof bundle_data);
    if (rc)
        goto fail;

    JSValue g = JS_GetGlobalObject(ctx);
    faker = JS_GetPropertyStr(ctx, g, "faker");
    JS_FreeValue(ctx, g);
    if (!JS_IsObject(faker)) {
        set_err("bundle did not define globalThis.faker");
        goto fail;
    }

    if (eval_static(DATE_SHIM_JS, sizeof DATE_SHIM_JS - 1, "date-shim", NULL))
        goto fail;
    if (eval_static(WELLFORMED_JS, sizeof WELLFORMED_JS - 1, "wellformed", &well_formed_fn))
        goto fail;

    JSValue names;
    if (eval_static(ALLOWLIST_JS, sizeof ALLOWLIST_JS - 1, "allowlist", &names))
        goto fail;
    JSValue lenv = JS_GetPropertyStr(ctx, names, "length");
    uint32_t n = 0;
    JS_ToUint32(ctx, &n, lenv);
    JS_FreeValue(ctx, lenv);
    allow = calloc(n, sizeof *allow);
    if (!allow)
        abort();
    for (uint32_t i = 0; i < n; i++) {
        JSValue e = JS_GetPropertyUint32(ctx, names, i);
        const char *s = JS_ToCString(ctx, e);
        if (!s)
            abort();
        allow[allow_len++] = strdup(s);
        JS_FreeCString(ctx, s);
        JS_FreeValue(ctx, e);
    }
    JS_FreeValue(ctx, names);
    qsort(allow, allow_len, sizeof *allow, cmp_names);

    JS_RunGC(rt);
    JS_SetMemoryLimit(rt, JS_MEMORY_LIMIT);
    JS_SetInterruptHandler(rt, interrupt_cb, NULL);
    in_init = 0;
    return;

fail:
    /* Wizer reports a trap; build.sh -DINIT_DEBUG prints last_err through WASI instead. */
#ifdef INIT_DEBUG
    fprintf(stderr, "wizer-initialize failed: %s\n", last_err);
#endif
    abort();
}

/* Mixes host entropy into Math.random's state. Faker's default seed is drawn from Math.random, so a
 * subsequent seed_random() gives each instance its own sequence. */
EXPORT("reseed_math_random")
void reseed_math_random(const uint8_t *p, uint32_t n)
{
    uint64_t s = ctx->random_state;
    for (uint32_t i = 0; i < n; i++) {
        s ^= (uint64_t)p[i] << ((i & 7) * 8);
        s = (s ^ (s >> 30)) * 0xbf58476d1ce4e5b9ull;
        s = (s ^ (s >> 27)) * 0x94d049bb133111ebull;
        s ^= s >> 31;
    }
    ctx->random_state = s ? s : 1;
}

/* faker.seed(n) with n as a JS number. Returns 0 on success. */
EXPORT("set_seed")
int32_t set_seed(double seed)
{
    JS_UpdateStackTop(rt);
    interrupted = 0;
    JSValue fn = JS_GetPropertyStr(ctx, faker, "seed");
    JSValue arg = JS_NewFloat64(ctx, seed);
    JSValue r = JS_Call(ctx, fn, faker, 1, &arg);
    JS_FreeValue(ctx, fn);
    JS_FreeValue(ctx, arg);
    if (JS_IsException(r)) {
        take_exception(last_err, sizeof last_err);
        return -1;
    }
    JS_FreeValue(ctx, r);
    return 0;
}

/* faker.seed() with no argument: Faker draws a fresh seed from Math.random. */
EXPORT("seed_random")
int32_t seed_random(void)
{
    JS_UpdateStackTop(rt);
    interrupted = 0;
    JSValue fn = JS_GetPropertyStr(ctx, faker, "seed");
    JSValue r = JS_Call(ctx, fn, faker, 0, NULL);
    JS_FreeValue(ctx, fn);
    if (JS_IsException(r)) {
        take_exception(last_err, sizeof last_err);
        return -1;
    }
    JS_FreeValue(ctx, r);
    return 0;
}

/* faker.setDefaultRefDate(ms since epoch). */
EXPORT("set_ref_date")
int32_t set_ref_date(double ms)
{
    JS_UpdateStackTop(rt);
    interrupted = 0;
    JSValue fn = JS_GetPropertyStr(ctx, faker, "setDefaultRefDate");
    JSValue arg = JS_NewFloat64(ctx, ms);
    JSValue r = JS_Call(ctx, fn, faker, 1, &arg);
    JS_FreeValue(ctx, fn);
    JS_FreeValue(ctx, arg);
    if (JS_IsException(r)) {
        take_exception(last_err, sizeof last_err);
        return -1;
    }
    JS_FreeValue(ctx, r);
    return 0;
}

/* Absolute monotonic deadline in the host's monotonic_ns clock; 0 disables. */
EXPORT("set_deadline")
void set_deadline(int64_t monotonic_ns)
{
    deadline_ns = monotonic_ns;
}

EXPORT("set_memory_limit")
void set_memory_limit(uint32_t bytes)
{
    JS_SetMemoryLimit(rt, bytes);
}

/* Converts v to a well-formed UTF-8 string and appends (u32 len, bytes) to b. */
static int append_result(Buf *b, JSValue v, char *msg, size_t msglen, int *status)
{
    JSValue s = JS_ToString(ctx, v);
    if (JS_IsException(s)) {
        *status = take_exception(msg, msglen);
        return -1;
    }
    JSValue wf = JS_Call(ctx, well_formed_fn, JS_UNDEFINED, 1, &s);
    JS_FreeValue(ctx, s);
    if (JS_IsException(wf)) {
        *status = take_exception(msg, msglen);
        return -1;
    }
    size_t n;
    const char *p = JS_ToCStringLen(ctx, &n, wf);
    JS_FreeValue(ctx, wf);
    if (!p) {
        *status = take_exception(msg, msglen);
        return -1;
    }
    int rc = buf_u32(b, (uint32_t)n);
    if (!rc)
        rc = buf_put(b, p, n);
    JS_FreeCString(ctx, p);
    if (rc) {
        *status = rc == -1 ? ST_RESULT_TOO_LARGE : ST_OUT_OF_MEMORY;
        snprintf(msg, msglen, "%s", rc == -1 ? "result exceeds size limit" : "out of memory");
        return -1;
    }
    return 0;
}

/*
 * Calls faker.<module>.<method>(arg) n times and returns the String()-coerced results.
 * path:  "module.method", must be in the allowlist.
 * args:  one JSON document, or args_len == 0 for a bare call. The buffer must be NUL-terminated at
 *        args[args_len] (alloc() guarantees this). Object arguments are re-parsed per iteration so n calls
 *        behave exactly like n separate calls even when a method mutates its argument.
 */
EXPORT("call")
uint32_t call(const char *path, uint32_t path_len, const char *args, uint32_t args_len, uint32_t n)
{
    Buf out = {0};
    char msg[512];
    int status = ST_OK;
    msg[0] = 0;

    JS_UpdateStackTop(rt);
    interrupted = 0;

    if (path_len == 0 || path_len > MAX_PATH_BYTES || !valid_path(path, path_len))
        return finish(&out, ST_INVALID_METHOD, "invalid faker method format");
    char key[MAX_PATH_BYTES + 1];
    memcpy(key, path, path_len);
    key[path_len] = 0;
    if (!bsearch(&(char *){key}, allow, allow_len, sizeof *allow, cmp_names))
        return finish(&out, ST_UNKNOWN_METHOD, "unknown faker method");
    char *dot = strchr(key, '.');
    *dot = 0;

    JSValue mod = JS_GetPropertyStr(ctx, faker, key);
    JSValue fn = JS_GetPropertyStr(ctx, mod, dot + 1);
    if (!JS_IsFunction(ctx, fn)) {
        JS_FreeValue(ctx, mod);
        JS_FreeValue(ctx, fn);
        return finish(&out, ST_INTERNAL, "allowlisted method is not a function");
    }

    JSValue arg = JS_UNDEFINED;
    int have_arg = args_len > 0, reparse = 0;
    if (have_arg) {
        arg = JS_ParseJSON(ctx, args, args_len, "<arg>");
        if (JS_IsException(arg)) {
            take_exception(msg, sizeof msg);
            JS_FreeValue(ctx, mod);
            JS_FreeValue(ctx, fn);
            return finish(&out, ST_INVALID_ARG, msg);
        }
        reparse = JS_IsObject(arg);
    }

    for (uint32_t i = 0; i < n; i++) {
        if (i > 0 && reparse) {
            JS_FreeValue(ctx, arg);
            arg = JS_ParseJSON(ctx, args, args_len, "<arg>");
            if (JS_IsException(arg)) {
                status = take_exception(msg, sizeof msg);
                break;
            }
        }
        JSValue r = JS_Call(ctx, fn, mod, have_arg ? 1 : 0, have_arg ? &arg : NULL);
        if (JS_IsException(r)) {
            status = take_exception(msg, sizeof msg);
            break;
        }
        int rc = append_result(&out, r, msg, sizeof msg, &status);
        JS_FreeValue(ctx, r);
        if (rc)
            break;
    }
    JS_FreeValue(ctx, arg);
    JS_FreeValue(ctx, mod);
    JS_FreeValue(ctx, fn);
    return finish(&out, (uint8_t)status, msg);
}

/* Allocates len+1 bytes; the extra byte is a NUL so JSON arguments satisfy JS_ParseJSON. */
EXPORT("alloc")
uint32_t wasm_alloc(uint32_t len)
{
    uint8_t *p = malloc((size_t)len + 1);
    if (!p)
        return 0;
    p[len] = 0;
    return (uint32_t)(uintptr_t)p;
}

/* Returns a pointer to (u32 len, bytes) describing the last failed set_* or init operation. */
EXPORT("last_error")
uint32_t last_error(void)
{
    static uint8_t buf[4 + sizeof last_err];
    uint32_t n = (uint32_t)strlen(last_err);
    memcpy(buf, &n, 4);
    memcpy(buf + 4, last_err, n);
    return (uint32_t)(uintptr_t)buf;
}

/* Introspection used by tests: number of allowlisted methods. */
EXPORT("allowlist_len")
uint32_t allowlist_len(void)
{
    return (uint32_t)allow_len;
}
