"""Extract Phosphor artwork at a named weight into a standalone SVG.

# PORT-TIME PARITY TOOL — DIES WITH `web/`

This reads `web/node_modules`, which nothing in a shipping build may do
(CLAUDE.md; `native/scripts/check-invariants.sh` rule 7 enforces it for
`native/crates/`). It is exempt for the same reason `scripts/gen-extract.ts`
is: its whole job is comparing against the reference app, it is **run by hand**
and never at build time, its output is committed, and it is deleted along with
`web/`.

Usage — `Icon:weight:output-name`, repeatable:

    python3 scripts/extract-phosphor-icons.py Lock:fill:lock-fill
    python3 scripts/extract-phosphor-icons.py Lock:regular:lock --check

`--check` re-extracts and compares against what is committed instead of
writing, which is how this tool is validated: every committed file must
round-trip byte for byte at its recorded weight. See
`native/assets/icons/PROVENANCE.md` for which weights ship and why.
"""
import re, sys, pathlib

DEFS = pathlib.Path("web/node_modules/@phosphor-icons/react/dist/defs")
OUT = pathlib.Path("native/assets/icons")

def block(source: str, weight: str) -> str:
    """The map entry for `weight`, up to the start of the next entry."""
    start = source.index(f'"{weight}",')
    rest = source[start:]
    # Entries are `[\n    "<name>",` — the next one ends this block.
    nxt = re.search(r'\n  \[\n    "', rest)
    return rest[: nxt.start()] if nxt else rest

def paths(chunk: str):
    """Every `d` in source order, each with its `opacity` if it carries one."""
    out = []
    for m in re.finditer(r'd:\s*"([^"]+)"((?:\s*,\s*\n?\s*opacity:\s*"([\d.]+)")?)', chunk):
        out.append((m.group(1), m.group(3)))
    return out

def svg(icon: str, weight: str) -> str:
    source = (DEFS / f"{icon}.es.js").read_text(encoding="utf-8")
    body = "".join(
        f'<path d="{d}"{f" opacity=\"{o}\"" if o else ""}/>' for d, o in paths(block(source, weight))
    )
    if not body:
        raise SystemExit(f"{icon}/{weight}: no paths extracted")
    return ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" '
            f'fill="currentColor">{body}</svg>')

if __name__ == "__main__":
    check = "--check" in sys.argv
    specs = [a for a in sys.argv[1:] if not a.startswith("--")]
    if not specs:
        raise SystemExit(__doc__)
    for spec in specs:
        icon, weight, name = spec.split(":")
        text = svg(icon, weight)
        target = OUT / f"{name}.svg"
        if check:
            print(f"{name}: {'SAME' if target.read_text(encoding='utf-8').strip()==text else 'DIFFERS'}")
        else:
            target.write_text(text + "\n", encoding="utf-8")
            print(f"wrote {target} ({len(text)} bytes)")
