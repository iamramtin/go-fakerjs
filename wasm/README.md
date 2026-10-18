# wasm/

`harness.c` is the C side of the module: QuickJS-NG, the Faker.js bundle, and a small export surface, built as a
WASI **reactor** (no `_start`). `build.sh en <out.wasm>` compiles it; `scripts/snapshot.sh` (`make snapshot`)
runs the whole pipeline (clang, wasm-opt, Wizer, wasm2go).

Every decision below is deliberate; the ones marked **deviation** differ from what an obvious default would
suggest.

## Sources and build

- **QuickJS-NG v0.17.0, commit `6d46d07d`**, fetched by `scripts/fetch-quickjs.sh` as the GitHub tarball of
  that commit. Only `quickjs.c`, `libregexp.c`, `libunicode.c` and `dtoa.c` are compiled. `quickjs-libc.c`
  (std/os modules, event loop) is not: the module has no filesystem, process, or timer surface.
- **Amalgamated.** `harness.c` does `#include "quickjs.c"` so it can reach `JSContext::random_state`
  (see *Math.random* below). QuickJS-NG's own `qjs-wasi-reactor.c` uses the same technique.
- `-O2 -DNDEBUG`, `-std=gnu23` (for `#embed`), `-funsigned-char` as upstream's CMake does,
  `-ffile-prefix-map` and `-fno-ident` so no host path reaches the binary.
- **Features are pinned**, not left to the compiler default: bulk-memory, sign-ext, nontrapping-fptoint,
  multivalue, mutable-globals; reference-types, SIMD, and atomics are off. Wizer does not support reference
  types. clang 23 additionally emits `extended-const` and `call-indirect-overlong` by default; wasm2go
  accepts both.
- The Faker bundle is embedded with C23 `#embed` (`-DBUNDLE_FILE=<path>`). No generator step.
- Imports left in the module: `fakerjs.monotonic_ns`, and WASI `clock_time_get`, `fd_write`, `fd_close`,
  `fd_fdstat_get`, `fd_seek` (libc stdio and clocks). Nothing else: no `environ_*`, no `random_get`, no
  `proc_exit`, no path or socket calls.

## Memory and stack

| Setting | Value | Why |
|---|---|---|
| wasm shadow stack | 2 MiB (`-z stack-size`) | twice the JS limit below, which leaves room for native frames |
| `JS_SetMaxStackSize` | 1 MiB | **consistent with the above**: QuickJS measures its own stack use and raises `RangeError`, so it must trip before the wasm stack is exhausted |
| `JS_SetMemoryLimit` | 160 MiB | bounds JS heap growth; breaches surface as `ErrOutOfMemory` |
| wasm max memory | 256 MiB (`--max-memory`) | hard bound; `memory.grow` fails beyond it |
| initial memory | not forced | Wizer writes the post-init size (about 6 MiB for en) |
| result size | 64 MiB per call | `ErrResultTooLarge` |

Stack depth is re-established on every export with `JS_UpdateStackTop`, because the stack top recorded
during Wizer's run is baked into the snapshot.

## Exports

All strings are `(ptr, len)` in module memory. `call` returns a pointer to a buffer the host frees with
`free`: `u32 payload_len, u8 status, payload`. Status `0` payload is `n x (u32 len, bytes)`; any other
status payload is a message. Status codes are in `harness.c` and mirrored in `engine.go`.

| Export | Purpose |
|---|---|
| `wizer-initialize` | **deviation: name.** Wizer 11.0.3's default init export is `wizer-initialize`, not the older `wizer.initialize` (`wizer --help`). Creates runtime and context, evaluates the bundle, builds the allowlist, stops. |
| `reseed_math_random(ptr,len)` | folds host entropy into `ctx->random_state` |
| `seed_random()` | `faker.seed()` with no argument: Faker draws a new seed from the (reseeded) `Math.random` |
| `set_seed(f64)` | `faker.seed(n)`; the host passes the int64 converted to float64 (round-to-nearest-even), exactly as V8 parses `faker.seed(<int64 literal>)` |
| `set_ref_date(f64 ms)` | `faker.setDefaultRefDate(ms)` |
| `call(path,len,args,len,n)` | allowlisted `module.method`, optional single JSON argument, n results each `String()`-coerced |
| `alloc(len)` / `free(ptr)` | `alloc` returns `len+1` bytes with a trailing NUL (`JS_ParseJSON` requires it) |
| `last_error()` | message of the last failed `set_*` / init step |
| `set_deadline(i64)` | absolute deadline on the host's monotonic clock; `0` clears it |
| `set_memory_limit(u32)` | test hook |
| `allowlist_len()` | test hook |

## Safety rules

- **No `eval` of host strings anywhere.** The only JavaScript the module compiles is static init code
  (allowlist builder, Date shim, `toWellFormed` wrapper) and the Faker bundle. Host input reaches
  JavaScript solely as a parsed JSON *value* (`JS_ParseJSON`) or a property lookup of an allowlisted name.
- **Allowlist.** At init a static script enumerates `faker`'s own module objects and their
  function-valued methods (own and prototype chain, excluding `Object.prototype` names, `constructor` and
  `_`-prefixed names). The names are copied into a sorted C array. `call` rejects anything not in it with
  `ST_UNKNOWN_METHOD` before any JavaScript runs, and re-validates the `module.method` grammar in C
  (defence in depth: the Go side validates too).
- **Arguments.** An object argument is re-parsed for every one of the `n` calls, so `GenerateN(m, a, n)`
  equals `n` separate `Generate(m, a)` calls even when a Faker method mutates its argument.
- **UTF-8.** Results pass through `String.prototype.toWellFormed` so lone surrogates become U+FFFD
  one-for-one (as V8's UTF-8 conversion does) instead of leaking CESU-style bytes.
- **Interrupt.** `JS_SetInterruptHandler` (QuickJS calls it every 10 000 opcodes) calls the host import
  `fakerjs.monotonic_ns` and compares it with the deadline held in the module. A fired deadline raises the
  uncatchable QuickJS "interrupted" error, reported as status `ST_INTERRUPTED`.

## Determinism of the snapshot (Wizer)

Wizer forbids host calls during the init function unless `--allow-wasi`, and a snapshot that read the clock
would not be reproducible anyway. QuickJS reads the clock while creating the context (it seeds
`Math.random` and the hash seed from `gettimeofday`). Therefore:

- `-Wl,--wrap=gettimeofday -Wl,--wrap=clock_gettime`: while `in_init` is set both return
  `2000-01-01T00:00:00Z`. After init they forward to the real WASI clock.
- `ctx->random_state` is set to a constant right after the context exists. Faker draws its *default* seed
  from `Math.random` while the bundle loads, so without this the snapshot would differ per build, and every
  engine would start from the same Faker seed.
- At run time the host calls `reseed_math_random` and then `seed_random` (seed 0) or `set_seed`.

Result: two clean builds produce byte-identical `.wasm`, `.go` and `.dat` files (`make snapshot-repro`), and two
engines built from the same snapshot do **not** share a random sequence (`TestSeedSemantics`).

## Bytecode round trip

The bundle is compiled, serialised with `JS_WRITE_OBJ_STRIP_SOURCE | JS_WRITE_OBJ_STRIP_DEBUG`, read back
and run. The retained functions carry neither the 0.4 MB bundle source text nor debug info,
and the embedded source array is zeroed after init so the snapshot does not store it twice. Stripping only
changes `Function.prototype.toString` and stack traces; the conformance suite is unaffected.

## Shims (deliberate deviations from QuickJS behaviour)

- **`Date.prototype.toString` / `toTimeString`** append ` (Coordinated Universal Time)`. V8 prints the time
  zone display name; QuickJS prints none. The module has no time zone database and always runs in UTC, so the
  shim reproduces V8's output for `TZ=UTC` exactly. It is what makes `date.recent` byte-exact against the
  golden data. It is **not** a general solution: a V8 embedding prints the *host's* zone name.
- Nothing else is polyfilled. In particular there is no `Intl` and `toLocaleString` ignores its options, which
  is the one known divergence (`finance.amount` with `autoFormat`), documented in
  `golden/known_divergences.yaml`.

## Not done: `wasm-opt --spill-pointers`

micropython-go runs this Binaryen pass because MicroPython's collector scans the C stack conservatively and
pointers held only in Wasm locals would be invisible to it. QuickJS uses reference counting plus a cycle
collector that traces from the runtime's own roots; it never scans native stack or locals. The pass is
therefore not applied (it also fails without an exported `__stack_pointer`). Evidence that nothing relies on
it: the Go-GC stress tests, and the [module GC-stress run](../FINDINGS.md#52-robustness).
