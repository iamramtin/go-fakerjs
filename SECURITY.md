# Security policy

## Reporting a vulnerability

Report privately through GitHub's private vulnerability reporting on this repository: open the **Security** tab
and choose **Report a vulnerability**. Do not open a public issue for a suspected vulnerability. This is a single-maintainer experimental project: acknowledgement and fixes are best effort.

## Threat model

go-fakerjs runs Faker.js inside a WebAssembly module, in-process, with no host access. The caller controls the
method name and the JSON argument. Those are the inputs the module must treat as hostile.

What the design enforces:

- **Method allowlist.** The module only dispatches the Faker.js methods it finds at initialisation, which are the
  262 recorded in `golden/api.json` for the pinned release. Anything else fails with `ClassUnknownMethod`. The Go side first checks the method grammar
  (`module.method`, ASCII letters and digits, at most 256 bytes); the module checks it again. Names such as
  `constructor`, `toString`, `__proto__` and `faker` are not allowlisted methods (tests B7 and the fuzzers).
- **Arguments are data.** Arguments are JSON, parsed by the module's JSON parser and passed to Faker as values.
  They are never evaluated as code. The Go side caps them at 4 MiB by default (`WithMaxArgBytes`).
- **Memory.** The module's JavaScript heap is limited to 160 MiB; the WebAssembly memory to 256 MiB (link flag
  `--max-memory`); each result to 64 MiB. Exceeding a limit returns an error and leaves the engine usable.
- **Time.** Each call has a deadline: 30 s by default, set with `WithTimeout`. The module checks an interrupt
  handler while it runs JavaScript, so a runaway call returns `ErrDeadline`. A zero or negative timeout
  disables the bound; do not do that for untrusted input.
- **Stack.** JavaScript recursion is capped at 1 MiB (`JS_SetMaxStackSize`) inside a 2 MiB wasm stack. Argument
  JSON nested 100,000 levels deep is rejected before it reaches the module (`invalid-argument: exceeded max
  depth`); 10,000 levels are accepted and handled (see the [resource bombs](FINDINGS.md#52-robustness)).
- **Host surface.** The module has no filesystem, network, or environment access. The WASI shim implements clocks
  and writes to stdout and stderr only; every other call returns ENOSYS. Retained module output is capped at
  64 KiB.
- **Panics.** A panic or trap inside the host import or the module poisons the engine, which then refuses further
  calls. `Pool` replaces poisoned engines.

What the design does not enforce, and where to look:

- **Faker's own templates.** `helpers.fake` and similar methods interpret template strings by design. An
  attacker-chosen template can cost time and memory up to the limits above, but cannot reach the host.
- **Known runaway methods.** Faker.js (found in 10.6.0) has a method that never returns for some arguments
  (`string.nanoid({length: n})`); the default timeout is what stops it. Relying on the deadline for untrusted
  input is the intended use, but a caller that sets no timeout will hang.
- **Memory after a limit error.** An engine that reached its memory limit keeps that memory until it is closed.
  Recycle engines after a resource-limit error.
- **Side channels, timing, and entropy.** Output is deterministic for a given seed. Do not use a seeded engine
  to produce secrets. A zero seed draws its entropy from `crypto/rand` through `Math.random`, but Faker is not
  a cryptographic generator, and this project makes no claim about its output being unpredictable.
- **Platform.** The module is [verified byte-for-byte](FINDINGS.md#53-build-and-reproducibility) on linux/arm64
  in this repository, and CI confirms the same golden dump on linux/amd64, darwin/arm64, and windows/amd64.

## Supported versions

Only the latest release, currently v0.1.0, receives fixes.
