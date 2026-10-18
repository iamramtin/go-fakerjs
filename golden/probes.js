// Semantic probes evaluated identically in a clean Node vm context (V8) and in QuickJS-NG.
// The expression is wrapped so that it returns a JSON object of name -> String(result) or "THROWS: msg".
(function () {
  var out = {};
  function p(name, fn) {
    try { out[name] = String(fn()); } catch (e) { out[name] = 'THROWS: ' + (e && e.name); }
  }
  p('typeof Intl', function () { return typeof Intl; });
  p('typeof TextEncoder', function () { return typeof TextEncoder; });
  p('typeof btoa', function () { return typeof btoa; });
  p('typeof structuredClone', function () { return typeof structuredClone; });
  p('typeof Temporal', function () { return typeof Temporal; });
  p('Date toString', function () { return new Date(0).toString(); });
  p('Date toUTCString', function () { return new Date(0).toUTCString(); });
  p('Date toISOString', function () { return new Date(1e12).toISOString(); });
  p('Date toDateString', function () { return new Date(1e12).toDateString(); });
  p('Date toTimeString', function () { return new Date(1e12).toTimeString(); });
  p('Date toLocaleString', function () { return new Date(1e12).toLocaleString(); });
  p('Date toLocaleDateString opts', function () { return new Date(1e12).toLocaleDateString('en-US', { weekday: 'long', month: 'long' }); });
  p('Date toLocaleString month long', function () { return new Date(1e12).toLocaleString('en-US', { month: 'long' }); });
  p('Date toLocaleTimeString', function () { return new Date(1e12).toLocaleTimeString('en-US'); });
  p('Date parse iso date', function () { return Date.parse('2020-01-01'); });
  p('Date parse iso local', function () { return Date.parse('2020-01-01T00:00:00'); });
  p('Date parse human', function () { return Date.parse('Jan 1, 2020'); });
  p('Date parse toString output', function () { return Date.parse(new Date(1e12).toString()); });
  p('Date getTimezoneOffset', function () { return new Date(1e12).getTimezoneOffset(); });
  p('Date invalid toString', function () { return new Date(NaN).toString(); });
  p('Date year 1900 toString', function () { return new Date(Date.UTC(1900, 5, 1)).toString(); });
  p('Date year 275000', function () { return new Date(8.64e15).toISOString(); });
  p('Number toLocaleString', function () { return (1234567.891).toLocaleString(); });
  p('Number toLocaleString minFrac', function () { return (20.55).toLocaleString(undefined, { minimumFractionDigits: 3 }); });
  p('Number toLocaleString en-US', function () { return (1234.5).toLocaleString('en-US'); });
  p('Number toLocaleString currency', function () { return (1234.5).toLocaleString('en-US', { style: 'currency', currency: 'USD' }); });
  p('RegExp lookbehind', function () { return /(?<=a)b/.exec('ab')[0]; });
  p('RegExp neg lookbehind', function () { return /(?<!a)b/.exec('ab cb').index; });
  p('RegExp unicode property', function () { return 'héllo wörld'.match(/\p{L}+/gu).join('|'); });
  p('RegExp unicode script', function () { return 'abc αβγ'.match(/\p{Script=Greek}+/u)[0]; });
  p('RegExp named groups', function () { return '2020-05'.replace(/(?<y>\d+)-(?<m>\d+)/, '$<m>/$<y>'); });
  p('RegExp dotAll', function () { return /a.b/s.test('a\nb'); });
  p('RegExp sticky', function () { var r = /a/y; r.lastIndex = 1; return r.test('ba'); });
  p('RegExp matchAll', function () { return Array.from('a1b2'.matchAll(/\d/g)).length; });
  p('RegExp case-insens unicode', function () { return /K/iu.test('k') + '/' + /K/i.test('k'); });
  p('RegExp \\w unicode i', function () { return /\w/iu.test('ſ'); });
  p('String replaceAll', function () { return 'aaa'.replaceAll('a', 'b'); });
  p('String normalize NFD', function () { return 'é'.normalize('NFD').length; });
  p('String normalize NFKC', function () { return 'ﬁ'.normalize('NFKC'); });
  p('String localeCompare', function () { return ['b', 'a', 'C', 'ä', 'z'].sort(function (a, b) { return a.localeCompare(b); }).join(''); });
  p('String localeCompare sign', function () { return 'a'.localeCompare('B') + ',' + 'a'.localeCompare('A'); });
  p('String toUpperCase unicode', function () { return 'straße ǆ ﬃ'.toUpperCase(); });
  p('String toLowerCase unicode', function () { return 'İ'.toLowerCase().length; });
  p('String toLocaleUpperCase tr', function () { return 'i'.toLocaleUpperCase('tr'); });
  p('Array sort stability', function () {
    var a = []; for (var i = 0; i < 2000; i++) a.push({ k: i % 3, i: i });
    a.sort(function (x, y) { return x.k - y.k; });
    for (var j = 1; j < a.length; j++) if (a[j - 1].k === a[j].k && a[j - 1].i > a[j].i) return 'unstable at ' + j;
    return 'stable';
  });
  p('Array sort default', function () { return [10, 9, 1, 'b', 'a', undefined, 2].sort().join(','); });
  p('Array sort inconsistent cmp', function () { return [5, 1, 4, 2, 3, 9, 8, 7, 6, 0, 11, 15, 13, 12, 14].sort(function () { return -1; }).join(','); });
  p('Array sort random cmp w/ seed', function () { var s = 7; var r = function () { s = (s * 16807) % 2147483647; return s / 2147483647 - 0.5; }; return Array.from({ length: 40 }, function (_, i) { return i; }).sort(function () { return r(); }).join(','); });
  p('toFixed 1.005', function () { return (1.005).toFixed(2); });
  p('toFixed 1e21', function () { return (1e21).toFixed(2); });
  p('toFixed 0.5', function () { return (0.5).toFixed(0) + ',' + (1.5).toFixed(0) + ',' + (2.5).toFixed(0) + ',' + (-0.5).toFixed(0); });
  p('toPrecision', function () { return (0.000001234).toPrecision(3) + ',' + (123456).toPrecision(2) + ',' + (1.45).toPrecision(2); });
  p('toExponential', function () { return (123.456).toExponential(2) + ',' + (0).toExponential(); });
  p('Number toString radix', function () { return (255).toString(16) + ',' + (0.5).toString(2) + ',' + (0.1).toString(3) + ',' + (1e21).toString(36) + ',' + (-255.5).toString(16); });
  p('Number to string', function () { return [0.1 + 0.2, 1e21, 1e-7, 123456789012345680000, 5e-324, 1.7976931348623157e308, -0, 100, 0.000001].join(' '); });
  p('parseFloat/parseInt', function () { return [parseFloat('3.14abc'), parseInt('0x1f'), parseInt('08'), parseFloat('1e3'), parseFloat('.5'), Number(''), Number(' 12 '), Number('1_0')].join(','); });
  p('Math trig', function () { return [Math.sin(1), Math.cos(1), Math.tan(1), Math.asin(0.5), Math.atan2(1, 2), Math.sinh(1), Math.exp(1), Math.log(10), Math.pow(2, 0.5), Math.cbrt(2), Math.hypot(3, 4.5), Math.expm1(1e-5), Math.log1p(1e-5)].join(' '); });
  p('Math trig 2', function () { var o = []; for (var i = 1; i < 60; i++) o.push(Math.sin(i * 0.37) + Math.cos(i * 1.1) + Math.atan(i / 7) + Math.asin(i / 61) + Math.sqrt(i) + Math.log(i + 0.5) + Math.exp(i / 30) + Math.pow(i, 1.37)); return o.join(' '); });
  p('Math round', function () { return [Math.round(2.5), Math.round(-2.5), Math.round(0.49999999999999994), Math.fround(5.5), Math.trunc(-4.7), Math.sign(-3), Math.clz32(1)].join(','); });
  p('Math.random range', function () { var r = Math.random(); return r >= 0 && r < 1; });
  p('JSON.stringify date', function () { return JSON.stringify({ d: new Date(0) }); });
  p('JSON.parse __proto__', function () { return Object.keys(JSON.parse('{"__proto__": 1, "a": 2}')).join(','); });
  p('Object key order', function () { return Object.keys({ b: 1, 2: 1, a: 1, 1: 1, '-1': 1 }).join(','); });
  p('Array.at/toSorted/findLast', function () { return [typeof [].at, typeof [].toSorted, typeof [].findLast, typeof Object.groupBy, typeof Array.fromAsync].join(','); });
  p('String.at/isWellFormed', function () { return [typeof ''.at, typeof ''.isWellFormed, typeof ''.replaceAll].join(','); });
  p('Error message TypeError', function () { try { null.x; } catch (e) { return e.message; } });
  p('Error message destructure', function () { try { (function (_a) { var b = _a.from; })(); } catch (e) { return e.message; } });
  p('Symbol.description', function () { return Symbol('x').description; });
  p('BigInt', function () { return (2n ** 70n).toString() + ',' + BigInt.asUintN(8, 257n) + ',' + (123456789012345678901234567890n * 3n); });
  p('BigInt toString radix', function () { return (2n ** 70n).toString(36); });
  p('Number.MAX_SAFE', function () { return Number.MAX_SAFE_INTEGER + ',' + (2 ** 53 + 1) + ',' + 9007199254740993; });
  p('string split limit', function () { return 'a,b,c'.split(',', 2).length + ',' + 'abc'.split('').length; });
  p('str localeCompare numeric', function () { return 'a10'.localeCompare('a2', undefined, { numeric: true }); });
  return JSON.stringify(out);
})()
