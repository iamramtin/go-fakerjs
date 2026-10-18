// Runs golden/probes.js in a clean vm context and stores the V8 baseline in reports/probes-v8.json.
import fs from 'node:fs';
import vm from 'node:vm';
import path from 'node:path';
import { here } from './lib.mjs';
const r = JSON.parse(vm.runInContext(fs.readFileSync(path.join(here, 'probes.js'), 'utf8'), vm.createContext({})));
fs.mkdirSync(path.join(here, '..', 'reports'), { recursive: true });
fs.writeFileSync(path.join(here, '..', 'reports', 'probes-v8.json'), JSON.stringify(r, null, 1));
console.log(Object.keys(r).length, 'probes recorded');
