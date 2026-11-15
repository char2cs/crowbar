#!/usr/bin/env bash
# Moves sibling unit-test files into their source file as an inline `#[cfg(test)] mod tests { ... }`.
# Usage: inline-rust-tests.sh [--report FILE] DIR...   (DIR is a crate or any tree of .rs files)
# Handles `#[cfg(test)] #[path = "x_tests.rs"] mod tests;` and the bare `#[cfg(test)] mod tests;` form
# (foo.rs -> foo/tests.rs). Anything else (shared test files, a second test module, inner attributes
# that cannot be nested) is left untouched and listed in the report. Run `cargo fmt` afterwards.
set -uo pipefail
report=/dev/stderr
dirs=()
while (($#)); do
  case $1 in
    --report) report=$2; shift 2 ;;
    *) dirs+=("$1"); shift ;;
  esac
done
((${#dirs[@]})) || { echo "usage: inline-rust-tests.sh [--report FILE] DIR..." >&2; exit 2; }
: >"$report"
exec python3 -I - "$report" "${dirs[@]}" <<'PY'
import os, re, sys

report_path, dirs = sys.argv[1], sys.argv[2:]
wiring = re.compile(
    r'^(?P<indent>[ \t]*)#\[cfg\(test\)\]\s*\n'
    r'(?:[ \t]*#\[path\s*=\s*"(?P<path>[^"]+)"\]\s*\n)?'
    r'[ \t]*(?:pub(?:\([^)]*\))? )?mod (?P<name>[A-Za-z_0-9]+);[ \t]*\n',
    re.M,
)
skipped, moved, removable = [], [], {}
sources = []
for root in dirs:
    for base, subdirs, files in os.walk(root):
        subdirs[:] = [d for d in subdirs if d not in ("target", "vendor", ".git", "node_modules")]
        sources += [os.path.join(base, f) for f in files if f.endswith(".rs") and not f.endswith("_tests.rs")]

uses = {}
for src in sources:
    text = open(src).read()
    for match in wiring.finditer(text):
        name = match.group("name")
        if name != "tests" and not name.endswith("_tests"):
            continue
        rel = match.group("path")
        if rel:
            test_path = os.path.normpath(os.path.join(os.path.dirname(src), rel))
        else:
            stem = os.path.splitext(src)[0]
            test_path = stem + "/tests.rs"
        uses.setdefault(test_path, []).append(src)

for test_path, owners in uses.items():
    if len(owners) > 1:
        skipped.append(f"{test_path}: shared by {', '.join(owners)}")
        continue
    src = owners[0]
    if not os.path.isfile(test_path):
        skipped.append(f"{src}: test file {test_path} not found")
        continue
    body = open(test_path).read()
    text = open(src).read()
    matches = [m for m in wiring.finditer(text) if m.group("name") == "tests" or m.group("name").endswith("_tests")]
    if len(matches) != 1:
        skipped.append(f"{src}: {len(matches)} test modules wired; migrate by hand")
        continue
    if re.search(r'^#!\[', body, re.M) and not body.lstrip().startswith("#!["):
        skipped.append(f"{src}: inner attribute in the middle of {test_path}")
        continue
    if re.search(r'^\s*mod tests\b', body, re.M):
        skipped.append(f"{src}: {test_path} already declares `mod tests`")
        continue
    match = matches[0]
    inner = body if body.endswith("\n") else body + "\n"
    replacement = f"{match.group('indent')}#[cfg(test)]\n{match.group('indent')}mod tests {{\n{inner}{match.group('indent')}}}\n"
    new_text = text[: match.start()] + replacement + text[match.end():]
    open(src, "w").write(new_text)
    removable[test_path] = src
    moved.append(f"{src} <- {test_path}")

for test_path in removable:
    os.remove(test_path)
    parent = os.path.dirname(test_path)
    if os.path.basename(test_path) == "tests.rs" and not os.listdir(parent):
        os.rmdir(parent)

with open(report_path, "w") as out:
    out.write(f"moved {len(moved)}\n")
    for line in moved:
        out.write(f"  ok   {line}\n")
    out.write(f"not migrated {len(skipped)}\n")
    for line in skipped:
        out.write(f"  SKIP {line}\n")
print(f"inline-rust-tests: moved {len(moved)}, skipped {len(skipped)} (see {report_path})")
sys.exit(1 if skipped else 0)
PY
