// Enumerates every faker module and function-valued method in the BUNDLE, using a clean vm context.
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { here, loadBundleContext } from './lib.mjs';

const ctx = loadBundleContext('en');
const api = vm.runInContext(
  `(function(){
    var objProto = Object.getOwnPropertyNames(Object.prototype);
    var out = {};
    Reflect.ownKeys(faker).forEach(function(k){
      var mod = faker[k];
      if (typeof k !== 'string' || k.charAt(0) === '_' || !mod || typeof mod !== 'object' || mod.faker !== faker) return;
      var names = {};
      var o = mod;
      while (o && o !== Object.prototype) {
        Reflect.ownKeys(o).forEach(function(n){
          if (typeof n !== 'string' || n.charAt(0) === '_' || n === 'constructor' || objProto.indexOf(n) >= 0) return;
          var d = Object.getOwnPropertyDescriptor(o, n);
          if (d && typeof d.value === 'function' && !(n in names)) names[n] = d.value.length;
        });
        o = Object.getPrototypeOf(o);
      }
      var list = Object.keys(names).sort();
      if (list.length) out[k] = list.map(function (n) { return { n: n, a: names[n] }; });
    });
    return JSON.stringify(out);
  })()`,
  ctx,
);
const detailed = JSON.parse(api);
const modules = Object.fromEntries(Object.entries(detailed).map(([k, v]) => [k, v.map((x) => x.n)]));
const arity = Object.fromEntries(Object.entries(detailed).flatMap(([k, v]) => v.map((x) => [`${k}.${x.n}`, x.a])));
const sorted = Object.fromEntries(Object.keys(modules).sort().map((k) => [k, modules[k]]));
const counts = Object.fromEntries(Object.entries(sorted).map(([k, v]) => [k, v.length]));
const total = Object.values(counts).reduce((a, b) => a + b, 0);
const bad = Object.entries(sorted).flatMap(([m, l]) => l.filter((n) => !/^[a-zA-Z][a-zA-Z0-9]*$/.test(n)).map((n) => `${m}.${n}`));
if (bad.length) console.error('WARNING: methods outside the method grammar:', bad);

const pkg = JSON.parse(fs.readFileSync(path.join(here, 'node_modules/@faker-js/faker/package.json'), 'utf8'));
fs.writeFileSync(path.join(here, 'api.json'), JSON.stringify({ fakerVersion: pkg.version, total, counts, modules: sorted, arity }, null, 1) + '\n');
console.log(`faker ${pkg.version}: ${Object.keys(sorted).length} modules, ${total} methods`);
console.table(counts);
