#!/usr/bin/env python3
"""Import-cycle and layer check (manifest rules 16, 17, 20). Usage: check-cycles.py rust CRATE_SRC_DIR... | ts WEB_ROOT [FILE...] | layer WEB_ROOT FILE...
Rust: every module file of a crate (nested directories included), edges from `use` trees and `crate::`/`self::`/`super::` paths in non-test code; a `pub use` of a own child is structural and not an edge.
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

def clean_rust(text):
    text = re.sub(r'/\*.*?\*/', ' ', text, flags=re.S)
    text = re.sub(r'r#*"(?:.|\n)*?"#*', '""', text)
    text = re.sub(r'"(?:\\.|[^"\\])*"', '""', text)
    return re.sub(r'//[^\n]*', '', text)

def split_top(text):
    parts, depth, cur = [], 0, ""
    for ch in text:
        if ch == "{": depth += 1
        elif ch == "}": depth -= 1
        if ch == "," and depth == 0:
            parts.append(cur); cur = ""
        else:
            cur += ch
    return parts + [cur]

def expand_use(tree):
    """Expands a use tree (`a::{b, c::{d, e}}`) into a list of segment lists."""
    tree = tree.strip()
    i = tree.find("{")
    if i == -1:
        tree = re.sub(r'\s+as\s+\w+$', '', tree)
        return [[x for x in re.split(r'\s*::\s*', tree) if x and x != "*"]]
    base = [x for x in re.split(r'\s*::\s*', tree[:i].strip()) if x]
    inner = tree[i + 1: tree.rindex("}")]
    return [base + sub for part in split_top(inner) if part.strip() for sub in expand_use(part)]

def module_path(rel):
    parts = rel[:-3].split(os.sep)
    if parts[-1] == "mod": parts.pop()
    return tuple(parts)

def rust_edges(cur, text, mods):
    """Returns the set of module paths that module `cur` depends on."""
    edges = set()
    def resolve(segs, is_use):
        segs = [s for i, s in enumerate(segs) if not (s == "self" and i > 0)]
        if not segs: return None
        head, rest = segs[0], segs[1:]
        if head == "crate": abs_path = rest
        elif head == "self": abs_path = list(cur) + rest
        elif head == "super":
            up = 1
            while rest and rest[0] == "super": up += 1; rest = rest[1:]
            abs_path = list(cur[:-up]) + rest if up <= len(cur) else rest
        elif is_use and (cur + (head,)) in mods: abs_path = list(cur) + segs
        else: return None
        for n in range(len(abs_path), 0, -1):
            if tuple(abs_path[:n]) in mods: return tuple(abs_path[:n])
        return None
    uses = list(re.finditer(r'(?m)^[ \t]*(pub(?:\([^)]*\))?[ \t]+)?use[ \t]+([^;]+);', text))
    for m in uses:
        reexport = bool(m.group(1))
        for segs in expand_use(re.sub(r'\s+', ' ', m.group(2))):
            tgt = resolve(segs, True)
            if tgt is None or tgt == cur: continue
            if reexport and tgt[: len(cur)] == cur: continue
            edges.add(tgt)
    rest = re.sub(r'(?m)^[ \t]*(pub(?:\([^)]*\))?[ \t]+)?use[ \t]+[^;]+;', '', text)
    for m in re.finditer(r'(?<![A-Za-z0-9_:])(crate|self|super)((?:::[A-Za-z_][A-Za-z0-9_]*)+)', rest):
        tgt = resolve([m.group(1)] + m.group(2).split("::")[1:], False)
        if tgt is not None and tgt != cur: edges.add(tgt)
    return edges

def rust(src_dirs):
    bad = 0
    for src in src_dirs:
        files = {}
        for base, dirs, names in os.walk(src):
            dirs[:] = sorted(d for d in dirs if d not in ("target", "bin", "tests", "test_support"))
            for n in names:
                rel = os.path.relpath(os.path.join(base, n), src)
                if n.endswith(".rs") and rel not in ("lib.rs", "main.rs", "build.rs"): files[module_path(rel)] = os.path.join(base, n)
        graph = {}
        for mod, path in files.items():
            text = clean_rust(strip_tests(open(path, errors="replace").read()))
            graph[mod] = rust_edges(mod, text, files)
        for comp in tarjan(graph):
            print(f"{src}:1: [cycle] module cycle: {' <-> '.join('::'.join(c) for c in comp)}"); bad = 1
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

LAYER_LIB = ("src/lib/", "src/sdk/", "src/utils/")
LAYER_UP = ("src/components/", "src/features/", "src/routes/")
IMPORT_SPEC = re.compile(r"""(?:\bfrom|\bimport|\brequire)\s*\(?\s*(['"])([^'"\n]+)\1""")

def ts_layer(root, files):
    """Layer rule on resolved import targets: relative, alias, quoted either way, dynamic import()."""
    bad = 0
    for rel in files:
        text = open(os.path.join(root, rel), errors="replace").read()
        for m in IMPORT_SPEC.finditer(text):
            spec = m.group(2)
            line = text.count("\n", 0, m.start()) + 1
            if spec.startswith("@/"): target = "src/" + spec[2:]
            elif spec.startswith("."): target = os.path.normpath(os.path.join(os.path.dirname(rel), spec))
            else: continue
            if spec.startswith("../../../"):
                print(f"{rel}:{line}: [layer] deep relative import: use the @/ alias"); bad = 1
            if rel.startswith(LAYER_LIB) and target.startswith(LAYER_UP):
                print(f"{rel}:{line}: [layer] lib/sdk never import components/features/routes"); bad = 1
            if "/stores/" in rel and target.startswith("src/components/"):
                print(f"{rel}:{line}: [layer] stores never import components/"); bad = 1
    return bad

if __name__ == "__main__":
    kind, rest = sys.argv[1], sys.argv[2:]
    if kind == "rust": sys.exit(rust(rest))
    sys.exit(ts_layer(rest[0], rest[1:]) if kind == "layer" else ts(rest[0], rest[1:]))
