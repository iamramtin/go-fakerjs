#!/usr/bin/env python3
"""Median ns/op per benchmark from raw `go test -bench` output, and the ratios quoted in FINDINGS.md.

Usage: scripts/bench-ratios.py <bench-all.txt> <bench-v8go.txt>

Reads only the ns/op column; the headers, PASS and ok lines are ignored. Prints a markdown table of medians
and the native-to-V8 ratios (native ns / V8 ns, so values above 1 mean native is slower).
"""
import os
import statistics
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from tables import parse  # noqa: E402  (one parser for raw benchmark output)


def medians(path):
    return {k: statistics.median(v['ns/op']) for k, v in parse(path).items() if v['ns/op']}


def main():
    med = {}
    for path in sys.argv[1:3]:
        med.update(medians(path))

    def ratio(a, b):
        if a in med and b in med:
            return f'{med[a] / med[b]:.1f}x'
        return '-'

    print('| benchmark | native | baseline-qjs | v8go script-per-call | v8go precompiled | native / v8go script |')
    print('|---|---|---|---|---|---|')
    for name in ['person.firstName', 'internet.email', 'location.streetAddress', 'lorem.sentence',
                 'string.uuid', 'number.int']:
        n = f'BenchmarkGenerateSingle/native/{name}'
        b = f'BenchmarkGenerateSingle/baseline-qjs/{name}'
        s = f'BenchmarkGenerateSingle/v8go-script-per-call/{name}'
        p = f'BenchmarkGenerateSingle/v8go-precompiled/{name}'
        cells = [med.get(k) for k in (n, b, s, p)]
        fmt = [f'{c / 1e3:.2f} us' if c is not None else '-' for c in cells]
        print(f'| GenerateSingle {name} | {fmt[0]} | {fmt[1]} | {fmt[2]} | {fmt[3]} | {ratio(n, s)} |')
    for mode in ['percall', 'batched']:
        n = f'BenchmarkRow20/native/{mode}'
        s = f'BenchmarkRow20/v8go-script-per-call/{mode}'
        p = f'BenchmarkRow20/v8go-precompiled/{mode}'
        b = f'BenchmarkRow20/baseline-qjs/{mode}'
        cells = [med.get(k) for k in (n, b, s, p)]
        fmt = [f'{c / 1e3:.1f} us' if c is not None else '-' for c in cells]
        print(f'| Row20 {mode} | {fmt[0]} | {fmt[1]} | {fmt[2]} | {fmt[3]} | {ratio(n, s)} |')
    n = 'BenchmarkNewEngine/native'
    s = 'BenchmarkNewEngine/v8go-script-per-call'
    p = 'BenchmarkNewEngine/v8go-precompiled'
    b = 'BenchmarkNewEngine/baseline-qjs'
    cells = [med.get(k) for k in (n, b, s, p)]
    fmt = [f'{c / 1e3:.0f} us' if c is not None else '-' for c in cells]
    print(f'| NewEngine | {fmt[0]} | {fmt[1]} | {fmt[2]} | {fmt[3]} | {ratio(n, s)} |')


if __name__ == '__main__':
    main()
