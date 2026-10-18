// Shared helpers for the golden oracle scripts.
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

export const here = path.dirname(fileURLToPath(import.meta.url));
export const bundlePath = (variant = 'en') => path.join(here, '..', 'bundle', `faker.${variant}.min.js`);

/** Runs the minified bundle in a fresh vm context that has no Node globals. */
export function loadBundleContext(variant = 'en') {
  const ctx = vm.createContext({});
  vm.runInContext(fs.readFileSync(bundlePath(variant), 'utf8'), ctx, { filename: `faker.${variant}.min.js` });
  return ctx;
}

export const REF_DATE = '2026-01-01T00:00:00.000Z';

/**
 * Evaluates one faker call inside a vm context the same way a V8-based embedding does:
 * `faker.<module>.<method>(<JSON arg>)` then String(result).
 * Returns {ok:true,v} or {ok:false,cls,msg}.
 */
export function evalCall(ctx, method, arg) {
  ctx.__m = method;
  ctx.__a = arg === undefined || arg === null ? null : JSON.stringify(arg);
  return vm.runInContext(
    `(function(){
      try {
        var parts = __m.split('.');
        var args = __a === null ? [] : [JSON.parse(__a)];
        var r = faker[parts[0]][parts[1]].apply(faker[parts[0]], args);
        return { ok: true, v: String(r) };
      } catch (e) {
        return { ok: false, cls: 'js-exception', msg: String(e && e.message) };
      }
    })()`,
    ctx,
  );
}
