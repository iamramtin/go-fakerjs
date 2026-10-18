#!/usr/bin/env python3
"""Turns raw conformance and benchmark output into the markdown tables quoted in FINDINGS.md.

Usage:
  scripts/tables.py conformance <dir-with-conformance-json>
  scripts/tables.py bench <apple-m5-dir>
  scripts/tables.py bombs <dir-with-bombs.json>

The Apple M5 directory supplies bench-all.txt and bench-v8go.txt: per-method, Row20 per call and batched,
Row20 with several engines, the public Pool, NewEngine, and the v8go rows.
"""
import collections
import json
import pathlib
import re
import statistics
import sys


def row(t):
    n = t["Exact"] + t["Normalized"] + t["Divergent"]
    return f'{n} | {t["Exact"]} | {t["Normalized"]} | {t["Divergent"]} | {100 * (t["Exact"] + t["Normalized"]) / n:.2f}%'


def conformance(d):
    for name in ("native-en", "baseline-qjs"):
        p = pathlib.Path(d) / f"conformance-{name}.json"
        if not p.exists():
            continue
        c = json.loads(p.read_text())
        print(f"\n#### {name}: by category\n")
        print("| category | cases | exact | normalized | divergent | pass |\n|---|---|---|---|---|---|")
        for k, t in sorted(c["byCategory"].items()):
            print(f"| {k} | {row(t)} |")
        print(f"\n#### {name}: per-method cases by module\n")
        print("| module | cases | exact | normalized | divergent | pass |\n|---|---|---|---|---|---|")
        for k, t in sorted(c["byModule"].items()):
            print(f"| {k} | {row(t)} |")


LINE = re.compile(r"^(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+(.*)$")
PAIR = re.compile(r"([\d.]+(?:e[+-]?\d+)?)\s+([A-Za-z/]+)")


def parse(path):
    res = collections.defaultdict(lambda: collections.defaultdict(list))
    for line in pathlib.Path(path).read_text().splitlines():
        m = LINE.match(line)
        if not m:
            continue
        for val, unit in PAIR.findall(m.group(3)):
            res[m.group(1)][unit].append(float(val))
    return res


def med(xs):
    return statistics.median(xs) if xs else float("nan")


def bench(apple):
    a = parse(pathlib.Path(apple) / "bench-all.txt")
    v = parse(pathlib.Path(apple) / "bench-v8go.txt")
    both = {**{k: x for k, x in a.items()}, **{k: x for k, x in v.items()}}

    def get(name, unit="ns/op"):
        return med(both[name][unit]) if name in both else float("nan")

    print("\n#### Cold start (Apple M5, median of 10)\n")
    print("| benchmark | ns/op | B/op | allocs/op |\n|---|---|---|---|")
    for n in sorted(k for k in both if k.startswith(("BenchmarkNewEngine", "BenchmarkNewFromSnapshot"))):
        print(f"| {n.replace('Benchmark', '')} | {get(n):,.0f} | {get(n, 'B/op'):,.0f} | {get(n, 'allocs/op'):,.0f} |")

    methods = ["person.firstName", "internet.email", "location.streetAddress", "lorem.sentence", "string.uuid", "number.int"]
    print("\n#### GenerateSingle (Apple M5, ns/op, median of 10)\n")
    print("| method | native | baseline-qjs | v8go script per call | v8go precompiled | native / v8go script |\n|---|---|---|---|---|---|")
    for m in methods:
        n_ = get(f"BenchmarkGenerateSingle/native/{m}")
        q = get(f"BenchmarkGenerateSingle/baseline-qjs/{m}")
        s = get(f"BenchmarkGenerateSingle/v8go-script-per-call/{m}")
        p = get(f"BenchmarkGenerateSingle/v8go-precompiled/{m}")
        print(f"| {m} | {n_:,.0f} | {q:,.0f} | {s:,.0f} | {p:,.0f} | {n_ / s:.1f}x |")

    print("\n#### GenerateN amortised ns per value (Apple M5, median of 10)\n")
    print("| method | n | native | baseline-qjs | qjs / native |\n|---|---|---|---|---|")
    for m in methods:
        for n in (1, 10, 100, 1000):
            x = get(f"BenchmarkGenerateN/native/{m}/n={n}", "ns/value")
            q = get(f"BenchmarkGenerateN/baseline-qjs/{m}/n={n}", "ns/value")
            print(f"| {m} | {n} | {x:,.0f} | {q:,.0f} | {q / x:.1f}x |")

    print("\n#### Row20 (Apple M5): one Generate per cell (percall) vs GenerateN per column (batched)\n")
    print("| engine | mode | rows/s | ns/row | B/op | allocs/op |\n|---|---|---|---|---|---|")
    for e in ("native", "baseline-qjs", "v8go-script-per-call", "v8go-precompiled"):
        for mode in ("percall", "batched"):
            name = f"BenchmarkRow20/{e}/{mode}"
            if name in both:
                ns = get(name)
                print(f"| {e} | {mode} | {1e9 / ns:,.0f} | {ns:,.0f} | {get(name, 'B/op'):,.0f} | {get(name, 'allocs/op'):,.0f} |")

    def rps(name):
        return med(both[name]["rows/s"]) if name in both else float("nan")

    # b.N rows are shared between the k workers, so the benchmark's own rows/s metric is the aggregate rate.
    print("\n#### Row20 with k engines on k goroutines (Apple M5, 10 cores, aggregate rows/s, median of 10)\n")
    print("| engine | mode | engines | rows/s | speed-up vs 1 engine |\n|---|---|---|---|---|")
    for e in ("native", "baseline-qjs", "v8go-script-per-call", "v8go-precompiled"):
        for mode in ("percall", "batched"):
            base = rps(f"BenchmarkRow20Parallel/{e}/{mode}/pool=1")
            for k in (1, 2, 4, 8, 10):
                name = f"BenchmarkRow20Parallel/{e}/{mode}/pool={k}"
                if name in both:
                    print(f"| {e} | {mode} | {k} | {rps(name):,.0f} | {rps(name) / base:.2f}x |")

    print("\n#### Row20 through the public Pool, native (Apple M5, aggregate rows/s, median of 10)\n")
    print("| mode | pool | rows/s | speed-up vs pool of 1 |\n|---|---|---|---|")
    for mode in ("percall", "batched"):
        base = rps(f"BenchmarkRow20Pool/{mode}/pool=1")
        for k in (1, 2, 4, 8):
            name = f"BenchmarkRow20Pool/{mode}/pool={k}"
            if name in both:
                print(f"| {mode} | {k} | {rps(name):,.0f} | {rps(name) / base:.2f}x |")


def bombs(d):
    rows = json.loads((pathlib.Path(d) / "bombs.json").read_text())
    print("| bomb | outcome | time (ms) | peak RSS (MB) | engine usable after |\n|---|---|---|---|---|")
    for r in rows:
        print(f"| {r['name']} | {r['outcome']} | {r['durationMs']} | {r['peakRssMB']} | {'yes' if r['engineUsableAfter'] else 'no'} |")


if __name__ == "__main__":
    which = sys.argv[1] if len(sys.argv) > 1 else ""
    if which == "conformance":
        conformance(sys.argv[2])
    elif which == "bench":
        bench(sys.argv[2])
    elif which == "bombs":
        bombs(sys.argv[2])
    else:
        sys.exit(__doc__)
