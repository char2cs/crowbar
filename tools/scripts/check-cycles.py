#!/usr/bin/env python3
"""Import-cycle check (manifest rule 20). Usage: check-cycles.py rust CRATE_SRC_DIR... | ts WEB_ROOT [FILE...]
Rust: modules of one crate, edges from `use crate::a::...` and `crate::a::` paths in non-test code.
TypeScript: files, edges from static imports resolved through the `@/` alias and relative paths;
with FILE arguments only cycles that contain one of those files are reported."""
import os, re, sys

def tarjan(graph):
    index, low, stack, on, out, counter = {}, {}, [], set(), [], [0]
    def visit(node):
        index[node] = low[node] = counter[0]; counter[0] += 1
        stack.append(node); on.add(node)
        for nxt in graph.get(node, ()):
            if nxt not in index:
                visit(nxt); low[node] = min(low[node], low[nxt])
            elif nxt in on:
                low[node] = min(low[node], index[nxt])
        if low[node] == index[node]:
            comp = []
            while True:
                top = stack.pop(); on.discard(top); comp.append(top)
                if top == node: break
            if len(comp) > 1: out.append(sorted(comp))
    sys.setrecursionlimit(10000)
    for node in list(graph):
        if node not in index: visit(node)
    return out

def strip_tests(text):
    cut = re.search(r'^#\[cfg\(test\)\]\s*\nmod tests\b', text, re.M)
    return text[: cut.start()] if cut else text

def rust(src_dirs):
    bad = 0
    for src in src_dirs:
        mods = {os.path.splitext(f)[0] for f in os.listdir(src) if f.endswith(".rs") and f not in ("lib.rs", "main.rs")}
        graph = {}
        for mod in mods:
            text = strip_tests(open(os.path.join(src, mod + ".rs")).read())
            deps = set(re.findall(r'\bcrate::([a-z_][a-z0-9_]*)', text)) & mods
            graph[mod] = deps - {mod}
        for comp in tarjan(graph):
            print(f"{src}:1: [cycle] module cycle: {' <-> '.join(comp)}"); bad = 1
    return bad

def ts(root, only):
    graph, files = {}, []
    for base, dirs, names in os.walk(os.path.join(root, "src")):
        dirs[:] = [d for d in dirs if d != "node_modules"]
        files += [os.path.join(base, n) for n in names if n.endswith((".ts", ".tsx")) and ".test." not in n and not n.endswith(".d.ts")]
    known = {os.path.normpath(f) for f in files}
    def resolve(frm, spec):
        if spec.startswith("@/"): path = os.path.join(root, "src", spec[2:])
        elif spec.startswith("."): path = os.path.join(os.path.dirname(frm), spec)
        else: return None
        path = os.path.normpath(path)
        for cand in (path, path + ".ts", path + ".tsx", os.path.join(path, "index.ts"), os.path.join(path, "index.tsx")):
            if cand in known: return cand
        return None
    for f in files:
        text = open(f, errors="replace").read()
        specs = re.findall(r'^(?:import|export)\s+(?!type\b)[^;\'"]*?from\s+[\'"]([^\'"]+)[\'"]', text, re.M)
        graph[os.path.normpath(f)] = {r for r in (resolve(f, s) for s in specs) if r and r != os.path.normpath(f)}
    only = {os.path.normpath(os.path.join(root, o)) for o in only}
    bad = 0
    for comp in tarjan(graph):
        if only and not only & set(comp): continue
        rel = [os.path.relpath(c, root) for c in comp]
        print(f"{rel[0]}:1: [cycle] import cycle: {' <-> '.join(rel[:6])}{' ...' if len(rel) > 6 else ''}"); bad = 1
    return bad

if __name__ == "__main__":
    kind, rest = sys.argv[1], sys.argv[2:]
    sys.exit(rust(rest) if kind == "rust" else ts(rest[0], rest[1:]))
