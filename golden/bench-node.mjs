// V8 proxy numbers. tommie/v8go could not be built in the spike environment, so this measures the same
// workload on Node's V8 (JIT enabled) in a clean vm context. Two call styles:
//   script  - runInContext("faker.m.f()") per value, like a per-call RunScript (compile + run)
//   direct  - a function call per value (the cheapest possible V8 embedding)
// NOT v8go: no cgo boundary, different V8 version, Node's own flags. Treat as an optimistic reference.
import vm from 'node:vm';
import { performance } from 'node:perf_hooks';
import { loadBundleContext, REF_DATE } from './lib.mjs';

const row20 = ['color.human', 'commerce.department', 'commerce.productDescription', 'commerce.productMaterial', 'commerce.productName', 'company.name', 'date.recent', 'date.timeZone', 'finance.currencyCode', 'internet.email', 'internet.url', 'location.city', 'location.country', 'location.state', 'location.streetAddress', 'location.zipCode', 'lorem.sentence', 'lorem.word', 'person.firstName', 'person.lastName'];
const single = ['person.firstName', 'internet.email', 'location.streetAddress', 'lorem.sentence', 'string.uuid', 'number.int'];

function fresh() {
  const ctx = loadBundleContext('en');
  vm.runInContext(`faker.seed(42); faker.setDefaultRefDate("${REF_DATE}")`, ctx);
  return ctx;
}
const results = [];
function time(name, perOp, fn, minMs = 1500) {
  fn(Math.max(1, Math.floor(perOp / 4))); // warm up
  let n = 0;
  const t0 = performance.now();
  while (performance.now() - t0 < minMs) { fn(perOp); n += perOp; }
  const ns = ((performance.now() - t0) * 1e6) / n;
  results.push({ name, nsPerOp: Math.round(ns) });
  console.log(`${name.padEnd(48)} ${Math.round(ns).toString().padStart(9)} ns/op`);
}

for (const m of single) {
  const ctx = fresh();
  const [a, b] = m.split('.');
  const fn = vm.runInContext(`(function(){ return faker.${a}.${b}(); })`, ctx);
  time(`GenerateSingle/node-direct/${m}`, 2000, (n) => { for (let i = 0; i < n; i++) fn(); });
  const script = new vm.Script(`faker.${a}.${b}()`);
  time(`GenerateSingle/node-script-cached/${m}`, 2000, (n) => { for (let i = 0; i < n; i++) script.runInContext(ctx); });
  time(`GenerateSingle/node-script-compile/${m}`, 500, (n) => { for (let i = 0; i < n; i++) vm.runInContext(`faker.${a}.${b}() /*${i}*/`, ctx); });
}
{
  const ctx = fresh();
  const fns = row20.map((m) => { const [a, b] = m.split('.'); return vm.runInContext(`(function(){ return faker.${a}.${b}(); })`, ctx); });
  time('Row20/node-direct (ns/row)', 200, (n) => { for (let i = 0; i < n; i++) { let s = 0; for (const f of fns) s += String(f()).length; } });
  const scripts = row20.map((m) => new vm.Script(`faker.${m}()`));
  time('Row20/node-script-cached (ns/row)', 200, (n) => { for (let i = 0; i < n; i++) { let s = 0; for (const sc of scripts) s += String(sc.runInContext(ctx)).length; } });
  let k = 0;
  time('Row20/node-script-compile (ns/row)', 100, (n) => { for (let i = 0; i < n; i++) { let s = 0; for (const m of row20) s += String(vm.runInContext(`faker.${m}() /*${k++}*/`, ctx)).length; } });
}
console.log(JSON.stringify(results));
