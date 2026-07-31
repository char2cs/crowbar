#!/usr/bin/env python3
"""Check `generated.rs` against the colours a real browser paints.

`gen-theme.py` re-implements the slice of CSS that `theme.css` uses: `var()`
chains, `oklch()`, `color-mix()`, `calc()`. A re-implementation is exactly the
kind of thing that is confidently wrong, so this script does not check it
against another transcription — it checks it against Chrome.

It builds a page from the *real* `theme.css` bytes, asks Chrome for the
computed value of every token in both appearances, paints each one to a canvas
so the answer comes back as the 8-bit sRGB the compositor would actually
produce, and diffs that against the HSLA constants in `generated.rs` (converted
back to sRGB the way gpui converts them at paint time).

    python3 crates/crowbar-ui/tools/verify-theme.py

Exits non-zero on the first channel that disagrees by more than one 8-bit step.
Requires Google Chrome; it is a verification tool, not part of the build.
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import importlib.util

_spec = importlib.util.spec_from_file_location(
    "gen_theme", Path(__file__).resolve().parent / "gen-theme.py"
)
gen = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(gen)

CHROME = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
GENERATED = gen.OUT

PAGE = """<!doctype html>
<meta charset="utf-8">
<style>
:root {{
{palette}
}}
{theme_css}
/* `@theme inline` and the file tree's own tokens are substituted where they are
   *used*, not computed once at the root: Tailwind inlines the former at build
   time, and the latter are declared on `.file-tree-item`, well inside whichever
   theme is in force. Declaring them on `*` reproduces that — a custom property
   computed at `:root` would inherit the light value into the dark subtree. */
* {{
{inlined}
{extras}
}}
</style>
<pre id="out"></pre>
<script>
const NAMES = {names};
const OTHERS = {others};
const canvas = document.createElement('canvas');
canvas.width = canvas.height = 1;
const ctx = canvas.getContext('2d', {{ willReadFrequently: true }});
// Composite each token over opaque white and opaque black rather than reading
// it back unpremultiplied: a low-alpha colour loses most of its precision in an
// 8-bit premultiplied buffer, and the two composites are what the token
// actually contributes to a frame anyway.
function over(resolved, backdrop) {{
  ctx.fillStyle = backdrop;
  ctx.fillRect(0, 0, 1, 1);
  ctx.fillStyle = resolved;
  ctx.fillRect(0, 0, 1, 1);
  const d = ctx.getImageData(0, 0, 1, 1).data;
  return [d[0], d[1], d[2]];
}}
function sample(name) {{
  const probe = document.createElement('span');
  probe.style.color = 'var(' + name + ')';
  document.body.appendChild(probe);
  const resolved = getComputedStyle(probe).color;
  document.body.removeChild(probe);
  return [over(resolved, '#ffffff'), over(resolved, '#000000')];
}}
// The app puts `.dark` on `document.documentElement`, so `:root` and `.dark`
// are the *same* element. That matters: a token declared only in `:root` but
// referencing one that `.dark` overrides (`--syntax-comment: var(--muted-
// foreground)`) picks up the dark value, which it would not if `.dark` were a
// descendant. Sampling is therefore two passes over one root, not two subtrees.
// Lengths, durations and font stacks are asked of the same engine, through
// whichever property gives the resolved value back in a canonical unit.
function sampleOther(name, kind) {{
  const probe = document.createElement('span');
  probe.style.position = 'absolute';
  probe.style.display = 'block';
  const v = 'var(' + name + ')';
  if (kind === 'Duration') {{
    probe.style.animation = v;
  }} else if (kind === 'FontFamily') {{
    probe.style.fontFamily = v;
  }} else if (kind === 'Scale') {{
    probe.style.width = 'calc(1000px * ' + v + ')';
  }} else if (name === '--app-scrollbar-thumb-border') {{
    probe.style.border = v;
  }} else {{
    probe.style.width = v;
  }}
  document.body.appendChild(probe);
  const cs = getComputedStyle(probe);
  let out;
  if (kind === 'Duration') {{
    out = cs.animationDuration;
  }} else if (kind === 'FontFamily') {{
    out = cs.fontFamily;
  }} else if (name === '--app-scrollbar-thumb-border') {{
    out = cs.borderTopWidth;
  }} else {{
    out = cs.width;
  }}
  document.body.removeChild(probe);
  return out;
}}
const out = {{}};
const other = {{}};
for (const name of NAMES) {{
  out[name] = {{ light: sample(name) }};
}}
for (const [name, kind] of Object.entries(OTHERS)) {{
  other[name] = sampleOther(name, kind);
}}
document.documentElement.classList.add('dark');
for (const name of NAMES) {{
  out[name].dark = sample(name);
}}
document.getElementById('out').textContent =
  JSON.stringify({{ colors: out, other: other }});
</script>
"""

# The emitted literals carry `_` digit separators (clippy::unreadable_literal);
# Python's `float()` accepts them verbatim.
F32 = r"[\d._]+"

# rustfmt breaks the wider entries across lines, so the pattern is whitespace
# tolerant rather than line-oriented.
HSLA = re.compile(
    rf"(\w+):\s*Color::seal\(Hsla\s*\{{\s*h:\s*({F32}),\s*s:\s*({F32}),"
    rf"\s*l:\s*({F32}),\s*a:\s*({F32}),?\s*\}}\)"
)


def hsl_to_rgb(h: float, s: float, lightness: float):
    """gpui's own `Hsla -> Rgba`, so the comparison is against what it paints."""
    c = (1.0 - abs(2.0 * lightness - 1.0)) * s
    x = c * (1.0 - abs((h * 6.0) % 2.0 - 1.0))
    m = lightness - c / 2.0
    sector = int(h * 6.0)
    table = {
        0: (c + m, x + m, m),
        6: (c + m, x + m, m),
        1: (x + m, c + m, m),
        2: (m, c + m, x + m),
        3: (m, x + m, c + m),
        4: (x + m, m, c + m),
    }
    r, g, b = table.get(sector, (c + m, m, x + m))
    return tuple(min(1.0, max(0.0, v)) for v in (r, g, b))


def parse_generated():
    """Read back the two tables from generated.rs, keyed by field name."""
    text = GENERATED.read_text()
    tables = {}
    for table, marker in (("light", "pub const LIGHT"), ("dark", "pub const DARK")):
        start = text.index(marker)
        end = text.index("};", start)
        tables[table] = {
            m.group(1): tuple(float(m.group(i)) for i in range(2, 6))
            for m in HSLA.finditer(text[start:end])
        }
    return tables


def build_page(names, others):
    palette = "\n".join(f"  {k}: {v};" for k, v in gen.TAILWIND_SRC.items())
    css = gen.strip_comments(gen.THEME_CSS.read_text())

    # `@theme inline` is a Tailwind at-rule: a browser ignores the whole block,
    # so its 70 declarations are lifted into `:root` verbatim. Only the custom
    # properties move — the `@keyframes` inside it are not tokens.
    # Non-greedy, and anchored on a `}` in column 0: the block's own closing
    # brace is the only unindented one, so the nested `@keyframes` do not end
    # the match early and the rest of the file does not get swept in.
    block = re.search(r"@theme inline \{(.*?)\n\}", css, re.S).group(1)
    inlined = "\n".join(
        f"  {m.group(1)}: {m.group(2)};"
        for m in (gen.DECL.match(line) for line in block.split("\n"))
        if m
    )
    css = css[: css.index("@theme inline")] + css[css.index(":root {") :]

    extras = "\n".join(
        f"  {name}: {value};"
        for name, value in gen.parse_file_tree_extras(gen.FILE_TREE_CSS)
    )
    return PAGE.format(
        palette=palette,
        theme_css=css,
        inlined=inlined,
        extras=extras,
        names=json.dumps(names),
        others=json.dumps(others),
    )


OTHER_PATTERNS = {
    "Space": re.compile(rf"(\w+):\s*Space::seal\(px\(({F32})\)\)"),
    "Radius": re.compile(rf"(\w+):\s*Radius::seal\(px\(({F32})\)\)"),
    "FontSize": re.compile(rf"(\w+):\s*FontSize::seal\(Rems\(({F32})\)\)"),
    "Duration": re.compile(
        r"(\w+):\s*Duration::seal\(StdDuration::from_millis\((\d+)\)\)"
    ),
    "FontFamily": re.compile(r"(\w+):\s*FontFamily::seal\(&\[(.*?)\]\)", re.S),
    "Scale": re.compile(rf"(\w+):\s*Scale::seal\(({F32})\)"),
}

# A whole number of seconds is emitted as `from_secs` (clippy insists); it is
# normalised back to milliseconds so the comparison has one unit.
SECONDS = re.compile(r"(\w+):\s*Duration::seal\(StdDuration::from_secs\((\d+)\)\)")


def parse_other():
    """Read the non-colour half of the LIGHT table back out of generated.rs."""
    text = GENERATED.read_text()
    start = text.index("pub const LIGHT")
    body = text[start : text.index("};", start)]
    out = {}
    for kind, pattern in OTHER_PATTERNS.items():
        for m in pattern.finditer(body):
            out[m.group(1)] = (kind, m.group(2))
    for m in SECONDS.finditer(body):
        out[m.group(1)] = ("Duration", str(int(m.group(2)) * 1000))
    return out


def check_other(field_to_css, ours, browser, failures):
    """Compare lengths, durations and font stacks against the same engine."""
    for field, (kind, raw) in ours.items():
        css_name = field_to_css[field]
        theirs = browser[css_name]
        if kind == "FontFamily":
            want = [f.strip().strip('"') for f in raw.split(",") if f.strip()]
            got = [f.strip().strip("'\"") for f in theirs.split(",") if f.strip()]
            # An empty stack means the token has no static value at all
            # (`--font-editor` is `var(--editor-font-family)` with no fallback);
            # Chrome falls back to its own default font, which is not a value
            # theme.css supplies. There is nothing to compare.
            if want and want != got:
                failures.append(f"  {css_name}: ours {want} chrome {got}")
        elif kind == "Duration":
            got = float(theirs.rstrip("s")) * 1000.0
            if abs(got - float(raw)) > 0.5:
                failures.append(f"  {css_name}: ours {raw}ms chrome {got}ms")
        elif kind == "Scale":
            got = float(theirs.rstrip("px")) / 1000.0
            if abs(got - float(raw)) > 1e-6:
                failures.append(f"  {css_name}: ours {raw} chrome {got}")
        else:
            want = float(raw) if kind != "FontSize" else float(raw) * 16.0
            got = float(theirs.rstrip("px"))
            if abs(got - want) > 0.01:
                failures.append(f"  {css_name}: ours {want}px chrome {got}px")


def main() -> int:
    tables = parse_generated()
    field_to_css = {}
    for line in GENERATED.read_text().split("\n"):
        m = re.match(r"\s*/// `(--[\w-]+)`:", line)
        if m:
            field_to_css[gen.field_name(m.group(1))] = m.group(1)

    other_fields = parse_other()
    if len(tables["light"]) + len(other_fields) != len(field_to_css):
        print(
            f"generated.rs has {len(field_to_css)} fields but only "
            f"{len(tables['light'])} colours + {len(other_fields)} others were "
            "parsed back — the emitter and this checker have drifted.",
            file=sys.stderr,
        )
        return 2

    color_fields = {f: c for f, c in field_to_css.items() if f in tables["light"]}
    names = [color_fields[f] for f in tables["light"]]
    others = {field_to_css[f]: kind for f, (kind, _) in other_fields.items()}
    page = build_page(names, others)

    with tempfile.TemporaryDirectory() as tmp:
        path = Path(tmp) / "probe.html"
        path.write_text(page)
        proc = subprocess.run(
            [
                CHROME,
                "--headless=new",
                "--disable-gpu",
                "--no-sandbox",
                "--allow-file-access-from-files",
                "--virtual-time-budget=4000",
                "--dump-dom",
                path.as_uri(),
            ],
            capture_output=True,
            text=True,
            timeout=180,
            check=False,
        )
    m = re.search(r'<pre id="out">(.*?)</pre>', proc.stdout, re.S)
    if not m:
        print(proc.stdout[-2000:], file=sys.stderr)
        print(proc.stderr[-2000:], file=sys.stderr)
        return 2
    browser = json.loads(m.group(1).replace("&amp;", "&").replace("&quot;", '"'))

    failures = []
    worst = 0
    for field, css_name in color_fields.items():
        for table in ("light", "dark"):
            h, s, lightness, a = tables[table][field]
            rgb = hsl_to_rgb(h, s, lightness)
            for backdrop, theirs in zip((1.0, 0.0), browser["colors"][css_name][table]):
                ours = [round((c * a + backdrop * (1.0 - a)) * 255) for c in rgb]
                delta = max(abs(x - y) for x, y in zip(ours, theirs))
                worst = max(worst, delta)
                if delta > 1:
                    over = "white" if backdrop else "black"
                    failures.append(
                        f"  {css_name} ({table}, over {over}): ours {ours} chrome {theirs}"
                    )

    check_other(field_to_css, other_fields, browser["other"], failures)

    checked = len(color_fields) * 4 + len(other_fields)
    if failures:
        print(f"{len(failures)} of {checked} samples disagree with Chrome:")
        print("\n".join(failures))
        return 1
    print(
        f"all {checked} samples agree with Chrome: {len(color_fields)} colour "
        f"tokens x light/dark x white/black backdrop (worst channel delta "
        f"{worst}/255), plus {len(other_fields)} lengths, durations, font stacks "
        "and scales"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
