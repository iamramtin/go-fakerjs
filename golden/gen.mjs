// Produces the golden oracle data and cross-checks three oracles:
//   O1  real npm @faker-js/faker (ESM, full package)
//   O2  the minified BUNDLE in a clean vm context           <- Go is compared against this one
//   O3  eval-per-call style: vm evaluating `faker.<m>(<JSON arg>)` then String(result)
// Usage: TZ=UTC node gen.mjs [--verify]
//   --verify  regenerate in memory and require byte-identical output to golden/testdata (and O1==O2).
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import zlib from 'node:zlib';
import crypto from 'node:crypto';
import { Buffer } from 'node:buffer';
import { here, loadBundleContext, REF_DATE } from './lib.mjs';

if (process.env.TZ !== 'UTC') {
  console.error('gen.mjs must run with TZ=UTC (Date.prototype.toString output depends on it)');
  process.exit(2);
}
const VERIFY = process.argv.includes('--verify');
const out = path.join(here, 'testdata');
const reports = path.join(here, '..', 'reports');
fs.mkdirSync(out, { recursive: true });
fs.mkdirSync(reports, { recursive: true });

const api = JSON.parse(fs.readFileSync(path.join(here, 'api.json'), 'utf8'));
const argsFile = JSON.parse(fs.readFileSync(process.env.ARGS_FILE ?? path.join(here, 'args.json'), 'utf8')).methods;
const METHOD_RE = /^[a-zA-Z][a-zA-Z0-9]*\.[a-zA-Z][a-zA-Z0-9]*$/;
const allowlist = new Set(Object.entries(api.modules).flatMap(([m, l]) => l.map((n) => `${m}.${n}`)));

// ---------------------------------------------------------------- oracles
const DRIVER_SRC = `(function (faker) {
  return function (m, aj) {
    try {
      var p = m.split('.');
      var mod = faker[p[0]];
      var args = aj === null ? [] : [JSON.parse(aj)];
      var r = mod[p[1]].apply(mod, args);
      return { v: String(r) };
    } catch (e) {
      return { e: 'js-exception' };
    }
  };
})`;

console.warn = () => {}; // silence faker deprecation notices in O1
// O1 runs in the main Node realm; strip the web-platform globals Node adds so it sees the same bare
// environment as the clean vm context (V8 embeddings and QuickJS provide none of these either).
for (const g of ['btoa', 'atob', 'TextEncoder', 'TextDecoder', 'structuredClone', 'URL', 'URLSearchParams', 'Buffer', 'crypto']) {
  try { delete globalThis[g]; } catch { /* non-configurable */ }
}
const { faker: o1faker } = await import('@faker-js/faker');
const o1driver = (0, eval)(DRIVER_SRC)(o1faker);

// One shared context, reseeded per case (faker.seed resets all generator state); a sample of cases is
// re-run in a brand-new context to prove that reseeding is equivalent to a fresh bundle load.
const sharedCtx = loadBundleContext('en');
const sharedDriver = vm.runInContext(DRIVER_SRC, sharedCtx)(sharedCtx.faker);
function newO2(seed, fresh = false) {
  const ctx = fresh ? loadBundleContext('en') : sharedCtx;
  const f = ctx.faker;
  f.seed(Number(seed));
  f.setDefaultRefDate(REF_DATE);
  return { call: fresh ? vm.runInContext(DRIVER_SRC, ctx)(f) : sharedDriver };
}
function newO1(seed) {
  o1faker.seed(Number(seed));
  o1faker.setDefaultRefDate(REF_DATE);
  return { call: o1driver };
}
/** O3: textual evaluation exactly like a V8-based embedding. Only used for cross-check on a sample. */
function runO3(seed, calls) {
  const ctx = sharedCtx;
  vm.runInContext(`faker.seed(${seed}); faker.setDefaultRefDate("${REF_DATE}")`, ctx);
  return calls.map(({ m, a }) => {
    try {
      return { v: String(vm.runInContext(`faker.${m}(${a === null ? '' : JSON.stringify(a)})`, ctx)) };
    } catch {
      return { e: 'js-exception' };
    }
  });
}

/** Contract-level error class for a method string, or null when the call should reach JS. */
function preclass(m) {
  if (m === '') return 'invalid-method';
  if (!METHOD_RE.test(m)) return 'invalid-method';
  if (!allowlist.has(m)) return 'unknown-method';
  return null;
}

function runCase(oracle, calls) {
  const res = [];
  for (const c of calls) {
    const n = c.n ?? 1;
    const aj = c.a === null || c.a === undefined ? null : JSON.stringify(c.a);
    const pre = preclass(c.m);
    for (let i = 0; i < n; i++) {
      if (pre) res.push({ e: pre });
      else res.push(oracle.call(c.m, aj));
    }
  }
  return res;
}
const compact = (r) => (r.v !== undefined ? r.v : { e: r.e });

// ---------------------------------------------------------------- case construction
const SEEDS = ['1', '42', '2147483647', '4294967296', '9007199254740991'];
const cases = { permethod: [], seededge: [], interleaved: [], workload22: [], coercion: [], errors: [] };

const missing = [];
const variantsFor = (m) => {
  const e = argsFile[m];
  if (Array.isArray(e)) return { variants: e, explicit: true };
  if (e && e.expected_error) return { variants: [null], explicit: true, expectedError: e.expected_error };
  return { variants: [null], explicit: false };
};
const stale = Object.keys(argsFile).filter((k) => !allowlist.has(k));
if (stale.length) {
  console.error('args.json lists methods that are not in api.json:', stale);
  process.exit(1);
}

for (const m of [...allowlist].sort()) {
  const { variants } = variantsFor(m);
  variants.forEach((a, vi) => {
    for (const seed of SEEDS) {
      cases.permethod.push({ id: `pm:${m}#${vi}@${seed}`, kind: 'per-method', seed, calls: [{ m, a, n: 200 }] });
    }
  });
}

const SEED_EDGE_SEEDS = ['-1', '9007199254740993', '9223372036854775807', '-9223372036854775808'];
const SEED_EDGE_METHODS = ['person.firstName', 'string.alpha', 'number.int', 'number.float', 'date.recent'];
for (const seed of SEED_EDGE_SEEDS) {
  for (const m of SEED_EDGE_METHODS) {
    cases.seededge.push({ id: `se:${m}@${seed}`, kind: 'seed-edge', seed, calls: [{ m, a: null, n: 20 }] });
  }
}

const WORKLOAD22 = [
  'color.human', 'commerce.department', 'commerce.productDescription', 'commerce.productMaterial', 'commerce.productName',
  'company.name', 'date.recent', 'date.timeZone', 'finance.currencyCode', 'internet.email', 'internet.url',
  'location.city', 'location.country', 'location.state', 'location.streetAddress', 'location.zipCode',
  'lorem.sentence', 'lorem.word', 'person.firstName', 'person.lastName', 'phone.number', 'string.alpha',
];
// The workload passes this options object for string.alpha.
const workloadArg = (m) => (m === 'string.alpha' ? { length: { min: 15, max: 15 } } : null);
const WORKLOAD_SEEDS = ['1', '2', '3', '42', '100', '12345', '65536', '2147483647', '4294967295', '987654321'];
for (const m of WORKLOAD22) {
  for (const seed of WORKLOAD_SEEDS) {
    cases.workload22.push({ id: `e22:${m}@${seed}`, kind: 'workload-22', seed, calls: [{ m, a: workloadArg(m), n: 1000 }] });
  }
}

// Interleaved: 22 workload paths + 40 other (method, arg) pairs chosen by a fixed rule, picked by a fixed PRNG.
function mulberry32(a) {
  return () => {
    a |= 0; a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
const probe = newO2('1');
const okPairs = [];
for (const m of [...allowlist].sort()) {
  if (WORKLOAD22.includes(m)) continue;
  const { variants } = variantsFor(m);
  for (const a of variants) {
    const r = probe.call(m, a === null ? null : JSON.stringify(a));
    if (r.v !== undefined) { okPairs.push({ m, a }); break; } // first succeeding variant of each method
  }
}
// Deterministic spread: every k-th pair, k derived from the pool size, capped at 40.
const step = Math.max(1, Math.floor(okPairs.length / 40));
const OTHER40 = okPairs.filter((_, i) => i % step === 0).slice(0, 40);
const POOL = [...WORKLOAD22.map((m) => ({ m, a: workloadArg(m) })), ...OTHER40];
[['7', 11], ['1234', 22], ['98765', 33]].forEach(([seed, prng], i) => {
  const rnd = mulberry32(prng);
  const calls = Array.from({ length: 5000 }, () => POOL[Math.floor(rnd() * POOL.length)]).map((p) => ({ m: p.m, a: p.a }));
  cases.interleaved.push({ id: `il:${i}@${seed}`, kind: 'interleaved', seed, calls });
});

const COERCION = [
  ['number.int', null], ['number.int', 100], ['number.float', null], ['datatype.boolean', null],
  ['helpers.arrayElements', ['a', 'b', 'c', 'd']], ['helpers.arrayElement', ['a', 1, null]],
  ['date.past', null], ['date.recent', null], ['date.future', null], ['date.soon', null], ['date.anytime', null],
  ['helpers.objectKey', { a: 1 }], ['helpers.objectValue', { a: 1 }], ['helpers.objectEntry', { a: 1 }],
  ['helpers.multiple', null], // array of undefined -> ",,"
  ['helpers.maybe', null], // callback absent: either String(undefined) or a TypeError, depending on the PRNG
  ['helpers.arrayElements', []], // empty array -> ""
  ['location.nearbyGPSCoordinate', null], ['number.bigInt', null], ['date.betweens', { from: '2020-01-01', to: '2020-12-31', count: 3 }],
  ['helpers.shuffle', [1, 2, 3, 4]], ['helpers.arrayElement', [[1, 2], { x: 1 }]], ['helpers.arrayElement', [null]],
  ['datatype.boolean', 0.5], ['number.float', { min: 1, max: 1, fractionDigits: 0 }], ['number.int', { min: 3, max: 3 }],
];
for (const seed of ['42', '1']) {
  COERCION.forEach(([m, a], i) => cases.coercion.push({ id: `co:${i}:${m}@${seed}`, kind: 'coercion', seed, calls: [{ m, a, n: 5 }] }));
}

const ERRORS = [
  ['', null], ['nomodule.method', null], ['person.nomethod', null], ['person.__proto__', null], ['person.constructor', null],
  ['person.toString', null], ['person.hasOwnProperty', null], ['person.valueOf', null], ['person.prototype', null],
  ['person.faker', null], ['helpers.constructor', null], ['person', null], ['person.', null], ['.x', null], ['1a.b', null],
  ['a.b.c', null], ['a.b()', null], ['person.firstName();globalThis.x=1', null], ['person.firstName\n', null],
  ['helpers.fake', '{{nonexistent.method}}'], ['helpers.fake', '{{person.nonexistent}}'], ['helpers.fake', 5],
  ['string.alpha', 'abc'], ['string.alpha', { length: 'x' }], ['string.alpha', { length: -1 }], ['number.int', 'x'],
  ['number.int', { min: 10, max: 1 }], ['helpers.arrayElement', 'abc'], ['helpers.arrayElement', []], ['helpers.arrayElement', 5],
  ['helpers.objectKey', []], ['date.between', null], ['date.between', { from: 'nonsense', to: 'nonsense' }],
  ['helpers.maybe', null], ['internet.jwt', null], ['person.firstName', [1, 2]], ['lorem.words', { min: 5, max: 1 }],
];
ERRORS.forEach(([m, a], i) => cases.errors.push({ id: `er:${i}`, kind: 'errors', seed: '42', calls: [{ m, a, n: 1 }] }));

// ---------------------------------------------------------------- evaluation
const mismatches = [];
const uncovered = [];
const allThrow = new Map(); // method -> {ok:boolean}
let o3checked = 0;
const o3mismatch = [];

let freshChecks = 0;
function evaluate(list) {
  let idx = 0; const T0 = Date.now();
  for (const c of list) {
    const r2 = runCase(newO2(c.seed), c.calls);
    if (idx % 300 === 0) console.error("  case", idx, "of", list.length, c.id, "heapMB", Math.round(process.memoryUsage().heapUsed/1e6), "s", Math.round((Date.now()-T0)/1000));
    if (idx++ % 97 === 0) {
      freshChecks++;
      const rf = runCase(newO2(c.seed, true), c.calls);
      if (JSON.stringify(rf) !== JSON.stringify(r2)) {
        console.error(`FAIL: reseeded shared context differs from a fresh context on case ${c.id}`);
        process.exit(1);
      }
    }
    const r1 = runCase(newO1(c.seed), c.calls);
    for (let i = 0; i < r2.length; i++) {
      if (JSON.stringify(r1[i]) !== JSON.stringify(r2[i])) {
        mismatches.push({ id: c.id, index: i, o1: r1[i], o2: r2[i] });
        break;
      }
    }
    if (c.kind === 'per-method') {
      const m = c.calls[0].m;
      const anyOk = r2.some((r) => r.v !== undefined);
      const prev = allThrow.get(m) ?? { ok: false };
      allThrow.set(m, { ok: prev.ok || anyOk });
    }
    c.r = r2.map(compact);
    c.o3 = undefined;
  }
}
for (const k of Object.keys(cases)) { evaluate(cases[k]); console.error(k, "done rss MB", Math.round(process.memoryUsage().rss/1e6), "heap", Math.round(process.memoryUsage().heapUsed/1e6)); }

// O3 cross-check on every non-per-method case plus a sample of per-method ones. For error cases O3 is
// informational (the eval-per-call reference has no allowlist, so person.toString succeeds there by design of the hardening).
const o3Info = [];
for (const c of [...cases.workload22.filter((_, i) => i % 7 === 0), ...cases.interleaved, ...cases.coercion, ...cases.seededge, ...cases.permethod.filter((_, i) => i % 41 === 0)]) {
  const calls = c.calls.flatMap((x) => Array.from({ length: x.n ?? 1 }, () => ({ m: x.m, a: x.a })));
  const r3 = runO3(c.seed, calls).map(compact);
  o3checked++;
  const bad = r3.findIndex((x, i) => JSON.stringify(x) !== JSON.stringify(c.r[i]));
  if (bad >= 0) o3mismatch.push({ id: c.id, index: bad, o3: r3[bad], o2: c.r[bad] });
}
for (const c of cases.errors) {
  const [x] = runO3(c.seed, c.calls.map((y) => ({ m: y.m, a: y.a })));
  const pre = preclass(c.calls[0].m);
  if (pre && x.v !== undefined) o3Info.push({ id: c.id, method: c.calls[0].m, contract: pre, v8_eval_equivalent: x.v });
}

// Fail loudly: methods that throw on every variant with no explicit entry.
const loud = [];
for (const m of allowlist) {
  const { explicit } = variantsFor(m);
  if (!explicit && !allThrow.get(m)?.ok) loud.push(m);
}
if (loud.length) {
  console.error(`\nFAIL: ${loud.length} enumerated method(s) throw on all variants and have no args.json entry or expected_error marker:\n  ${loud.join('\n  ')}`);
  console.error('Add explicit argument variants (or an expected_error marker) to golden/args.json.');
  process.exit(1);
}

// ---------------------------------------------------------------- write / verify
const gz = (lines) => zlib.gzipSync(Buffer.from(lines.map((l) => JSON.stringify(l)).join('\n') + '\n'), { level: 9 });
const files = {};
for (const [k, list] of Object.entries(cases)) {
  files[`${k}.jsonl.gz`] = gz(list.map(({ o3, ...rest }) => rest));
}
const callCount = Object.fromEntries(Object.entries(cases).map(([k, l]) => [k, l.reduce((s, c) => s + c.r.length, 0)]));
const variantsTotal = cases.permethod.length / SEEDS.length;
const meta = {
  fakerVersion: api.fakerVersion,
  nodeVersion: process.version,
  refDate: REF_DATE,
  tz: process.env.TZ,
  methods: allowlist.size,
  perMethodVariants: variantsTotal,
  cases: Object.fromEntries(Object.entries(cases).map(([k, l]) => [k, l.length])),
  calls: callCount,
  o1EqualsO2: mismatches.length === 0,
  o1o2Mismatches: mismatches.length,
  freshContextChecks: freshChecks,
  o3Checked: o3checked,
  o3Mismatches: o3mismatch.length,
  workload22: WORKLOAD22,
  interleavedPool: POOL.map((p) => p.m),
  contractVsV8Eval: o3Info,
  sha256: Object.fromEntries(Object.entries(files).map(([k, b]) => [k, crypto.createHash('sha256').update(b).digest('hex')])),
};

fs.writeFileSync(path.join(reports, 'golden-bundle-fidelity.json'), JSON.stringify({ mismatches, o3mismatch }, null, 1));
console.log(`methods=${allowlist.size} variants=${variantsTotal} calls=${JSON.stringify(callCount)}`);
console.log(`O1==O2: ${meta.o1EqualsO2} (${mismatches.length} case mismatches)  O3 sample: ${o3checked} cases, ${o3mismatch.length} mismatches`);
if (mismatches.length) console.log('first O1/O2 mismatches:', JSON.stringify(mismatches.slice(0, 5)));
if (o3mismatch.length) console.log('first O3 mismatches:', JSON.stringify(o3mismatch.slice(0, 5)));

if (VERIFY) {
  let ok = true;
  for (const [name, buf] of Object.entries(files)) {
    const cur = fs.existsSync(path.join(out, name)) ? fs.readFileSync(path.join(out, name)) : null;
    if (!cur || !cur.equals(buf)) { console.error(`verify: ${name} differs from stored golden data`); ok = false; }
  }
  if (!meta.o1EqualsO2) { console.error('verify: bundle fidelity O1==O2 failed'); ok = false; }
  process.exit(ok ? 0 : 1);
}
for (const [name, buf] of Object.entries(files)) fs.writeFileSync(path.join(out, name), buf);
fs.writeFileSync(path.join(out, 'meta.json'), JSON.stringify(meta, null, 1) + '\n');
console.log('wrote', Object.keys(files).join(' '), '+ meta.json');
