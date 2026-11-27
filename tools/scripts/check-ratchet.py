#!/usr/bin/env python3
"""Ratchet filter for the rule checkers (docs/standards-coverage.md, "Baselines").

Reads findings `path:line: [rule] message` on stdin and prints the ones that fail the build.
Rules in RATCHET are compared per (file, rule) with the committed baseline `tools/scripts/<lang>-baseline.txt`
(`allow-lint` uses `allow-baseline.txt`, one key per lint name): a count above the baseline fails with every
finding of that group listed, a count below it fails as stale until the baseline is shrunk
(`--shrink`). Every other finding passes through unchanged. Baselines never grow: `--guard OLD NEW`
fails when NEW has a key or count above OLD. `--strict` ignores the baselines (the merge gate).

Usage: check-ratchet.py --lang rust|ts|swift [--scope PREFIX]... [--scanned FILE] [--strict] [--shrink] [--baseline-dir DIR]
       check-ratchet.py --seed --lang L ...   (initial seeding only; writes the baseline from stdin)
       check-ratchet.py --guard OLD NEW   |   check-ratchet.py --self-test
       check-ratchet.py --rekey --lang L OLD NEW   (findings on stdin; a pure file move: see `rekey`)"""
import contextlib, os, re, sys, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
COMMON = {"param-count", "param-layout", "bool-param", "magic-number", "magic-string", "named-return",
          "spec-ref", "platform-ref", "tag-reason"}
RATCHET = {
    "rust": COMMON | {"real-infra", "table-driven", "test-missing", "panic", "cycle", "result-string", "map-err-ignore",
                      "swallowed-error", "direct-time", "direct-fs", "trait-size", "fn-count", "allow-lint"},
    "ts": COMMON | {"layer", "real-infra", "table-driven", "test-missing"},
    "swift": COMMON | {"real-infra", "table-driven", "test-missing"},
}
SUBKEYED = {"allow-lint"}
LINE = re.compile(r"^(?P<file>[^\s:][^:]*):(?P<line>\d+): \[(?P<rule>[a-z-]+)\] (?P<msg>.*)$")

def baseline_path(lang, rule, directory):
    return os.path.join(directory, "allow-baseline.txt" if rule in SUBKEYED else f"{lang}-baseline.txt")

def read_baseline(path):
    table = {}
    if not os.path.exists(path): return table
    for raw in open(path):
        raw = raw.rstrip("\n")
        if not raw or raw.startswith("#"): continue
        file, key, count = raw.split("\t")
        table[(file, key)] = int(count)
    return table

def write_baseline(path, table, header):
    with open(path, "w") as out:
        out.write(header)
        for (file, key), count in sorted(table.items()):
            if count > 0: out.write(f"{file}\t{key}\t{count}\n")

def key_of(finding):
    rule = finding["rule"]
    if rule in SUBKEYED: return rule + ":" + finding["msg"].split(": ", 1)[0]
    return rule

def parse(lines):
    findings, passthrough = [], []
    for raw in lines:
        m = LINE.match(raw.rstrip("\n"))
        if m: findings.append({"file": m["file"], "line": int(m["line"]), "rule": m["rule"], "msg": m["msg"], "raw": raw.rstrip("\n")})
        elif raw.strip(): passthrough.append(raw.rstrip("\n"))
    return findings, passthrough

def in_scope(file, scopes, scanned):
    if scanned is not None and file not in scanned: return False
    return not scopes or any(file.startswith(s) for s in scopes)

def run(lang, lines, directory, strict, shrink, scopes, scanned, seed=False):
    findings, passthrough = parse(lines)
    ratchet = RATCHET[lang]
    out = list(passthrough)
    groups = {}
    for f in findings:
        if f["rule"] not in ratchet: out.append(f["raw"]); continue
        groups.setdefault(f["rule"] in SUBKEYED and "allow" or "main", {}).setdefault((f["file"], key_of(f)), []).append(f)
    for kind, name in (("main", f"{lang}-baseline.txt"), ("allow", "allow-baseline.txt")):
        path = os.path.join(directory, name)
        base, current = read_baseline(path), groups.get(kind, {})
        updated, stale = dict(base), []
        for key, items in sorted(current.items()):
            allowed = 0 if strict else base.get(key, 0)
            if seed: updated[key] = len(items); continue
            if len(items) > allowed:
                out.extend(i["raw"] for i in items)
                out.append(f"{key[0]}:1: [baseline] {key[1]}: {len(items)} findings, baseline allows {allowed}")
            elif len(items) < allowed:
                updated[key] = len(items)
                stale.append(f"{key[0]}:1: [baseline-stale] {key[1]}: baseline {allowed} > actual {len(items)}: run the checker with --shrink")
        for key, allowed in sorted(base.items()):
            if key in current or not in_scope(key[0], scopes, scanned): continue
            updated[key] = 0
            stale.append(f"{key[0]}:1: [baseline-stale] {key[1]}: baseline {allowed} > actual 0: run the checker with --shrink")
        if strict and not seed: continue
        if (shrink or seed) and updated != base:
            header = f"# Ratchet baseline for {lang}: path<TAB>rule<TAB>count. Counts only shrink; never add or raise an entry.\n"
            if kind == "allow": header = "# Lint suppressions: path<TAB>allow-lint:<lint><TAB>count. Counts only shrink; never add or raise an entry.\n"
            write_baseline(path, updated, header)
            stale = []
        out.extend(stale)
    return out

def guard(old, new):
    a, b, bad = read_baseline(old), read_baseline(new), 0
    for key, count in sorted(b.items()):
        if count > a.get(key, 0):
            print(f"{new}: [baseline-grew] {key[0]} {key[1]}: {a.get(key, 0)} -> {count}; baselines only shrink"); bad = 1
    return bad

def rekey(lang, lines, directory, old, new):
    """Moves the baseline entries of file OLD to file NEW after a pure move of the file.
    Refuses (returns the reasons, changes nothing) when NEW already has an entry, when OLD still
    has findings, or when NEW has more findings of a rule than OLD was allowed: a rekey never
    grows a baseline. Entries are written with the findings NEW actually has, so a move that also
    fixed some findings shrinks them."""
    findings, _ = parse(lines)
    actual = {}
    for f in findings:
        if f["rule"] in RATCHET[lang]: actual[(f["file"], key_of(f))] = actual.get((f["file"], key_of(f)), 0) + 1
    problems, moves = [], {}
    for kind, name in (("main", f"{lang}-baseline.txt"), ("allow", "allow-baseline.txt")):
        path = os.path.join(directory, name)
        base = read_baseline(path)
        for (file, key), allowed in base.items():
            if file != old: continue
            if (new, key) in base: problems.append(f"{new}: [rekey] already has a baseline entry for {key}")
            if actual.get((old, key), 0): problems.append(f"{old}: [rekey] still has {key} findings: not a pure move")
            if actual.get((new, key), 0) > allowed: problems.append(f"{new}: [rekey] {key}: {actual[(new, key)]} findings, {old} was allowed {allowed}")
            moves.setdefault(path, []).append((key, actual.get((new, key), 0)))
    for key, count in actual.items():
        if key[0] == new and not any(key[1] == k for entries in moves.values() for k, _ in entries): problems.append(f"{new}: [rekey] {key[1]}: {count} findings with no baseline entry on {old}")
    if problems or not moves: return problems or [f"{old}: [rekey] no baseline entry to move"]
    for path, entries in moves.items():
        table = read_baseline(path)
        for key, count in entries: del table[(old, key)]; table[(new, key)] = count
        write_baseline(path, table, open(path).readline())
    return []

def self_test():
    rc = 0
    with tempfile.TemporaryDirectory() as d:
        open(os.path.join(d, "rust-baseline.txt"), "w").write("a.rs\tmagic-number\t2\n")
        mk = lambda rule, n, msg="m": [f"a.rs:{i + 1}: [{rule}] {msg}" for i in range(n)]
        cases = [
            ("within baseline passes", mk("magic-number", 2), True, {}),
            ("above baseline fails", mk("magic-number", 3), False, {}),
            ("below baseline is stale", mk("magic-number", 1), False, {}),
            ("new file with a ratcheted rule fails", ["b.rs:1: [fn-count] n"], False, {}),
            ("hard rule always fails", ["a.rs:1: [depth] d"], False, {}),
            ("allow-lint within baseline", ["a.rs:1: [allow-lint] clippy::x: s"], True, {"extra": mk("magic-number", 2)}),
            ("allow-lint other lint fails", ["a.rs:1: [allow-lint] clippy::y: s"] + mk("magic-number", 2), False, {}),
            ("strict ignores baseline", mk("magic-number", 2), False, {"strict": True}),
        ]
        for name, lines, want_ok, opt in cases:
            lines = lines + opt.get("extra", [])
            allow = os.path.join(d, "allow-baseline.txt")
            open(allow, "w").write("a.rs\tallow-lint:clippy::x\t1\n" if "extra" in opt else "")
            res = run("rust", lines, d, opt.get("strict", False), False, [], None)
            ok = (not res) == want_ok
            print(("ok   " if ok else "FAIL ") + "ratchet: " + name)
            rc |= 0 if ok else 1
        res = run("rust", mk("magic-number", 1), d, False, True, [], None)
        shrunk = read_baseline(os.path.join(d, "rust-baseline.txt")).get(("a.rs", "magic-number"))
        ok = shrunk == 1 and not res
        print(("ok   " if ok else "FAIL ") + "ratchet: --shrink lowers the count"); rc |= 0 if ok else 1
        old, new = os.path.join(d, "old.txt"), os.path.join(d, "new.txt")
        open(old, "w").write("a.rs\tmagic-number\t1\n"); open(new, "w").write("a.rs\tmagic-number\t2\n")
        with open(os.devnull, "w") as sink, contextlib.redirect_stdout(sink): ok = guard(old, new) == 1
        print(("ok   " if ok else "FAIL ") + "ratchet: --guard rejects a grown baseline"); rc |= 0 if ok else 1
        rc |= rekey_self_test(d)
    return rc

def rekey_self_test(d):
    rc = 0
    path = os.path.join(d, "rust-baseline.txt")
    mk = lambda file, rule, n: [f"{file}:{i + 1}: [{rule}] m" for i in range(n)]
    cases = [
        ("rekey keeps a count that does not grow", 2, mk("b.rs", "magic-number", 2), True, 2),
        ("rekey lowers a count the move fixed", 2, mk("b.rs", "magic-number", 1), True, 1),
        ("rekey refuses growth", 2, mk("b.rs", "magic-number", 3), False, 2),
        ("rekey refuses a file that still has findings", 2, mk("a.rs", "magic-number", 1) + mk("b.rs", "magic-number", 1), False, 2),
        ("rekey refuses findings of a rule it never allowed", 2, mk("b.rs", "magic-number", 1) + mk("b.rs", "fn-count", 1), False, 2),
    ]
    for name, base, lines, want_ok, count in cases:
        open(path, "w").write(f"# header\na.rs\tmagic-number\t{base}\n")
        problems = rekey("rust", lines, d, "a.rs", "b.rs")
        table = read_baseline(path)
        moved = table.get(("b.rs", "magic-number")) == count and ("a.rs", "magic-number") not in table
        kept = table.get(("a.rs", "magic-number")) == base and ("b.rs", "magic-number") not in table
        ok = (not problems) == want_ok and (moved if want_ok else kept)
        print(("ok   " if ok else "FAIL ") + "ratchet: " + name); rc |= 0 if ok else 1
    return rc

def main(argv):
    if "--self-test" in argv: return self_test()
    if "--guard" in argv:
        i = argv.index("--guard"); return guard(argv[i + 1], argv[i + 2])
    if "--rekey" in argv:
        i = argv.index("--rekey"); lang = argv[argv.index("--lang") + 1]
        out = rekey(lang, sys.stdin.readlines(), HERE, argv[i + 1], argv[i + 2])
        print("\n".join(out)) if out else None
        return 1 if out else 0
    opts = {"--lang": None, "--baseline-dir": HERE, "--scanned": None}
    scopes, flags, i = [], set(), 0
    while i < len(argv):
        a = argv[i]
        if a == "--scope": scopes.append(argv[i + 1]); i += 1
        elif a in opts: opts[a] = argv[i + 1]; i += 1
        else: flags.add(a)
        i += 1
    scanned = set(open(opts["--scanned"]).read().split("\n")) if opts["--scanned"] else None
    out = run(opts["--lang"], sys.stdin.readlines(), opts["--baseline-dir"], "--strict" in flags, "--shrink" in flags, scopes, scanned, "--seed" in flags)
    print("\n".join(out)) if out else None
    return 1 if out else 0

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
