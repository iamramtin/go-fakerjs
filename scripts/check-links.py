#!/usr/bin/env python3
"""Checks internal links in every Markdown file of the repository.

- A relative link must name a file or directory that exists.
- A link with an anchor (file.md#heading or #heading) must match a heading in the target, slugged the way
  GitHub does it.
- Every Markdown file must be reachable from README.md in two clicks or fewer.

External (http, https, mailto) links are not fetched. Exit status 1 lists every failure.
"""
import collections
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LINK = re.compile(r'(?<!!)\[(?:[^\]\\]|\\.)*\]\(([^)\s]+)(?:\s+"[^"]*")?\)')
HEADING = re.compile(r'^(#{1,6})\s+(.*?)\s*#*\s*$')
FENCE = re.compile(r'^\s*(```|~~~)')


def markdown_files():
    out = subprocess.run(['git', 'ls-files', '--cached', '--others', '--exclude-standard', '*.md'],
                         cwd=ROOT, capture_output=True, text=True, check=True).stdout.split()
    return sorted(f for f in out if 'node_modules/' not in f)


def strip_code(lines):
    """Yields (line number, text) outside fenced blocks, with inline code removed."""
    fenced = False
    for i, line in enumerate(lines, 1):
        if FENCE.match(line):
            fenced = not fenced
            continue
        if not fenced:
            yield i, re.sub(r'`[^`]*`', '', line)


def slug(text):
    text = re.sub(r'<[^>]+>', '', text)                    # inline HTML, including marker comments
    text = re.sub(r'\[([^\]]*)\]\([^)]*\)', r'\1', text)   # link text only
    text = text.replace('`', '').strip().lower()
    text = re.sub(r'[^\w\- ]', '', text)
    return text.replace(' ', '-')


def anchors(path):
    seen = collections.Counter()
    result = set()
    with open(path, encoding='utf-8') as f:
        lines = f.read().split('\n')
    fenced = False
    for line in lines:
        if FENCE.match(line):
            fenced = not fenced
            continue
        m = None if fenced else HEADING.match(line)
        if m:
            s = slug(m.group(2))
            result.add(s if seen[s] == 0 else f'{s}-{seen[s]}')
            seen[s] += 1
    return result


def links(path):
    with open(os.path.join(ROOT, path), encoding='utf-8') as f:
        lines = f.read().split('\n')
    for n, text in strip_code(lines):
        for m in LINK.finditer(text):
            yield n, m.group(1)


def resolve(src, target):
    """Returns (absolute path or None for same file, anchor) for an internal link, or None if external."""
    if re.match(r'^[a-z][a-z0-9+.-]*:', target):
        return None
    path, _, anchor = target.partition('#')
    if not path:
        return os.path.join(ROOT, src), anchor
    return os.path.normpath(os.path.join(ROOT, os.path.dirname(src), path)), anchor


def main():
    files = markdown_files()
    errors = []
    graph = collections.defaultdict(set)
    cache = {}
    for src in files:
        for n, target in links(src):
            r = resolve(src, target)
            if r is None:
                continue
            dest, anchor = r
            if not os.path.exists(dest):
                errors.append(f'{src}:{n}: {target}: no such file')
                continue
            if os.path.isdir(dest) and os.path.exists(os.path.join(dest, 'README.md')):
                dest = os.path.join(dest, 'README.md')
            rel = os.path.relpath(dest, ROOT)
            if rel.endswith('.md'):
                graph[src].add(rel)
            if anchor:
                if not rel.endswith('.md'):
                    errors.append(f'{src}:{n}: {target}: anchor on a non-Markdown file')
                    continue
                if dest not in cache:
                    cache[dest] = anchors(dest)
                if anchor not in cache[dest]:
                    errors.append(f'{src}:{n}: {target}: no heading with anchor #{anchor}')

    depth = {'README.md': 0}
    queue = collections.deque(['README.md'])
    while queue:
        cur = queue.popleft()
        for nxt in graph[cur]:
            if nxt not in depth:
                depth[nxt] = depth[cur] + 1
                queue.append(nxt)
    for f in files:
        if depth.get(f, 99) > 2:
            errors.append(f'{f}: not reachable from README.md in two clicks or fewer')

    for e in errors:
        print(e)
    if errors:
        print(f'check-links: {len(errors)} problem(s)')
        return 1
    print(f'check-links: ok ({len(files)} Markdown files, {sum(len(v) for v in graph.values())} internal doc links)')
    return 0


if __name__ == '__main__':
    sys.exit(main())
