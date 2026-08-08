#!/usr/bin/env python3
"""Generate `src/theme/generated.rs` from the React app's `web/src/styles/theme.css`.

PORT-TIME TOOL — requires `web/` to exist, and dies with it.

This script's entire job is parity against the React app: it reads
`web/src/styles/theme.css` (and file-explorer-tree.css) and transcribes their
resolved values into Rust. That makes a `web/` reference correct here, not a
defect to fix — S0.7's item is about `native/` *shipping* without `web/`, and
this tool is not part of what ships (see `nothing in a shipping build depends
on it`, below). Once `web/` is deleted, this script stops running and this
file should be deleted alongside it; nothing else in `native/` calls it.

**Nothing in a shipping build depends on it.** This script's *output*,
`crates/crowbar-ui/src/theme/generated.rs`, is committed to the repository —
`cargo build`/`cargo test`/`cargo clippy` read that committed file and never
invoke Python, so a checkout with no `web/` and no Python interpreter builds
and tests identically. Re-run this script by hand only when `theme.css`
itself changes and `generated.rs` needs to be regenerated to match; nothing in
the Rust toolchain does that automatically.

The port's design tokens are not re-authored: they are the *same* tokens the
React app ships, resolved once at generation time into concrete values. This
script is the transcription, so that nobody hand-copies 180 colours and gets one
of them wrong.

What it does:

  1. Parses `web/src/styles/theme.css` into its three declaration blocks
     (`@theme inline`, `:root`, `.dark`) plus the three extra tokens the file
     tree declares in its own stylesheet.
  2. Loads the Tailwind v4 default palette (`--color-neutral-800` and friends),
     which theme.css references but does not declare.
  3. Resolves every token twice — once with `:root` in force (light) and once
     with `.dark` layered on top — following `var()` chains, `oklch()`,
     `color-mix()` and `calc()` exactly as a browser would.
  4. Emits a `Theme` struct with one field per token and two `const` tables.

Run from anywhere:

    python3 crates/crowbar-ui/tools/gen-theme.py

It writes `crates/crowbar-ui/src/theme/generated.rs` and prints a summary. It is
deliberately assertive: any declaration it cannot account for, any `var()` it
cannot resolve, and any drift in the measured counts is a hard failure rather
than a silently skipped token.
"""

from __future__ import annotations

import math
import re
import struct
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve()
UI_CRATE = HERE.parent.parent
NATIVE = UI_CRATE.parent.parent
REPO = NATIVE.parent

# The **vendored** CSS under `native/assets/theme/`, not `web/`'s copy. See
# that directory's PROVENANCE.md: the CSS is the token source of truth and has
# to outlive `web/`, or the tokens survive only as Rust literals nobody can
# re-derive. `--check-vendored` diffs the two while `web/` still exists.
THEME_CSS = REPO / "native/assets/theme/theme.css"
FILE_TREE_CSS = REPO / "native/assets/theme/file-explorer-tree.css"

UPSTREAM = {
    THEME_CSS: REPO / "web/src/styles/theme.css",
    FILE_TREE_CSS: REPO / "web/src/features/file-explorer/styles/file-explorer-tree.css",
}


def check_vendored() -> int:
    """Diff the vendored CSS against `web/`'s, while `web/` still exists."""
    drifted = 0
    for vendored, upstream in UPSTREAM.items():
        if not upstream.exists():
            print(f"{upstream.relative_to(REPO)}: gone — vendored copy is now the only source")
            continue
        if vendored.read_bytes() == upstream.read_bytes():
            print(f"{vendored.relative_to(REPO)}: matches upstream")
        else:
            print(f"{vendored.relative_to(REPO)}: DIFFERS from {upstream.relative_to(REPO)}")
            drifted += 1
    return drifted
OUT = UI_CRATE / "src/theme/generated.rs"

# The measured shape of theme.css, from the item brief. Drift here means the
# React tokens moved and this generator's output is stale, so it is an error.
EXPECTED_DECL_LINES = 254
EXPECTED_DISTINCT = 180
EXPECTED_DUAL = 74

# ---------------------------------------------------------------------------
# The Tailwind v4 default palette.
# ---------------------------------------------------------------------------
# theme.css references these but never declares them; they come from
# `tailwindcss/theme.css`. Only the entries actually referenced are listed. Each
# carries Tailwind's own published hex so `check_palette()` can prove the
# oklch -> sRGB pipeline below reproduces the colour the browser paints.
TAILWIND = {
    "--color-white": ("#fff", "#ffffff"),
    "--color-neutral-50": ("oklch(98.5% 0 0)", "#fafafa"),
    "--color-neutral-100": ("oklch(97% 0 0)", "#f5f5f5"),
    "--color-neutral-400": ("oklch(70.8% 0 0)", "#a1a1a1"),
    "--color-neutral-500": ("oklch(55.6% 0 0)", "#737373"),
    "--color-neutral-800": ("oklch(26.9% 0 0)", "#262626"),
    "--color-red-400": ("oklch(70.4% 0.191 22.216)", "#ff6467"),
    "--color-red-500": ("oklch(63.7% 0.237 25.331)", "#fb2c36"),
    "--color-red-700": ("oklch(50.5% 0.213 27.518)", "#c10007"),
    "--color-orange-600": ("oklch(64.6% 0.222 41.116)", "#f54a00"),
    "--color-amber-400": ("oklch(82.8% 0.189 84.429)", "#ffb900"),
    "--color-amber-500": ("oklch(76.9% 0.188 70.08)", "#fe9a00"),
    "--color-amber-700": ("oklch(55.5% 0.163 48.998)", "#bb4d00"),
    "--color-lime-400": ("oklch(84.1% 0.238 128.85)", "#9ae600"),
    "--color-lime-600": ("oklch(64.8% 0.2 131.684)", "#5ea500"),
    "--color-emerald-400": ("oklch(76.5% 0.177 163.223)", "#00d492"),
    "--color-emerald-500": ("oklch(69.6% 0.17 162.48)", "#00bc7d"),
    "--color-emerald-700": ("oklch(50.8% 0.118 165.612)", "#007a55"),
    "--color-teal-600": ("oklch(60% 0.118 184.704)", "#009689"),
    "--color-cyan-900": ("oklch(39.8% 0.07 227.392)", "#104e64"),
    "--color-sky-400": ("oklch(74.6% 0.16 232.661)", "#00bcff"),
    "--color-sky-600": ("oklch(58.8% 0.158 241.966)", "#0084d1"),
    "--color-blue-400": ("oklch(70.7% 0.165 254.624)", "#51a2ff"),
    "--color-blue-500": ("oklch(62.3% 0.214 259.815)", "#2b7fff"),
    "--color-blue-700": ("oklch(48.8% 0.243 264.376)", "#1447e6"),
    "--color-purple-500": ("oklch(62.7% 0.265 303.9)", "#ad46ff"),
    "--color-rose-500": ("oklch(64.5% 0.246 16.439)", "#ff2056"),
}

# ---------------------------------------------------------------------------
# Colour maths.
# ---------------------------------------------------------------------------


def srgb_encode(x: float) -> float:
    """Linear-light sRGB channel -> gamma-encoded sRGB channel."""
    if x <= 0.0031308:
        return 12.92 * x
    return 1.055 * (x ** (1.0 / 2.4)) - 0.055


def oklch_to_srgb(lightness: float, chroma: float, hue_deg: float):
    """OKLCh -> gamma-encoded sRGB, clipped to gamut.

    The matrices are the ones in CSS Color 4 §9.3. Out-of-gamut results are
    clipped per channel; `check_palette()` proves that is what the browser does
    for every colour this codebase actually uses.
    """
    if chroma == 0.0:
        # Zero chroma is neutral by definition. Going through the matrices would
        # leave ~1e-9 of float noise between the channels, which is invisible but
        # turns into a garbage hue once the result is converted to HSL.
        gray = min(1.0, max(0.0, srgb_encode(lightness**3)))
        return (gray, gray, gray)

    hue = math.radians(hue_deg)
    a = chroma * math.cos(hue)
    b = chroma * math.sin(hue)

    l_ = lightness + 0.3963377774 * a + 0.2158037573 * b
    m_ = lightness - 0.1055613458 * a - 0.0638541728 * b
    s_ = lightness - 0.0894841775 * a - 1.2914855480 * b

    l3, m3, s3 = l_**3, m_**3, s_**3

    r = 4.0767416621 * l3 - 3.3077115913 * m3 + 0.2309699292 * s3
    g = -1.2684380046 * l3 + 2.6097574011 * m3 - 0.3413193965 * s3
    bl = -0.0041960863 * l3 - 0.7034186147 * m3 + 1.7076147010 * s3

    return tuple(min(1.0, max(0.0, srgb_encode(c))) for c in (r, g, bl))


def srgb_to_hsl(r: float, g: float, b: float):
    """Gamma-encoded sRGB -> HSL, all components normalised to 0..1.

    gpui's `Hsla` stores hue as a 0..1 turn, not degrees.
    """
    hi, lo = max(r, g, b), min(r, g, b)
    lightness = (hi + lo) / 2.0
    if hi == lo:
        return 0.0, 0.0, lightness
    delta = hi - lo
    sat = delta / (2.0 - hi - lo) if lightness > 0.5 else delta / (hi + lo)
    if hi == r:
        hue = (g - b) / delta + (6.0 if g < b else 0.0)
    elif hi == g:
        hue = (b - r) / delta + 2.0
    else:
        hue = (r - g) / delta + 4.0
    return hue / 6.0, sat, lightness


def check_palette() -> None:
    """Prove the oklch pipeline against Tailwind's own published hex values."""
    for name, (src, want_hex) in TAILWIND.items():
        got = parse_color(src)
        want = parse_color(want_hex)
        for i, chan in enumerate("rgb"):
            a = round(got[i] * 255)
            b = round(want[i] * 255)
            if abs(a - b) > 1:
                raise SystemExit(
                    f"palette check failed: {name} channel {chan}: "
                    f"got {a}, Tailwind publishes {b} ({src} vs {want_hex})"
                )


# ---------------------------------------------------------------------------
# CSS value parsing.
# ---------------------------------------------------------------------------

NUM = r"[-+]?(?:\d*\.\d+|\d+\.?\d*)"


def split_top_level(text: str, sep: str = ","):
    """Split on `sep`, ignoring separators nested inside parentheses."""
    out, depth, cur = [], 0, ""
    for ch in text:
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
        if ch == sep and depth == 0:
            out.append(cur.strip())
            cur = ""
        else:
            cur += ch
    out.append(cur.strip())
    return out


def parse_alpha(text: str) -> float:
    text = text.strip()
    if text.endswith("%"):
        return float(text[:-1]) / 100.0
    return float(text)


def parse_color(value: str):
    """Parse a *literal* colour (no `var()`, no `color-mix`) to RGBA 0..1."""
    value = value.strip()
    if value == "transparent":
        return (0.0, 0.0, 0.0, 0.0)
    if value.startswith("#"):
        hexpart = value[1:]
        if len(hexpart) in (3, 4):
            hexpart = "".join(c * 2 for c in hexpart)
        chans = [int(hexpart[i : i + 2], 16) / 255.0 for i in range(0, len(hexpart), 2)]
        if len(chans) == 3:
            chans.append(1.0)
        return tuple(chans)
    m = re.fullmatch(r"oklch\(\s*(.+?)\s*\)", value, re.S)
    if m:
        body = m.group(1)
        alpha = 1.0
        if "/" in body:
            body, alpha_txt = body.split("/", 1)
            alpha = parse_alpha(alpha_txt)
        parts = body.split()
        if len(parts) != 3:
            raise ValueError(f"oklch needs 3 components: {value!r}")
        lightness = (
            float(parts[0][:-1]) / 100.0 if parts[0].endswith("%") else float(parts[0])
        )
        chroma = (
            float(parts[1][:-1]) / 100.0 * 0.4
            if parts[1].endswith("%")
            else float(parts[1])
        )
        hue = float(parts[2])
        r, g, b = oklch_to_srgb(lightness, chroma, hue)
        return (r, g, b, alpha)
    raise ValueError(f"not a literal colour: {value!r}")


def color_mix(space: str, c1, p1: float, c2, p2: float):
    """CSS Color 5 `color-mix()`, premultiplied.

    Only the two spaces this codebase uses are implemented: `srgb` (mix the
    gamma-encoded channels directly) and `oklch`. For `oklch` every mix in the
    tree is against `transparent`, which — because premultiplication zeroes its
    contribution and its hue is powerless — is exactly "scale the alpha", so it
    is handled without a round trip through OKLCh.
    """
    total = p1 + p2
    if total <= 0:
        raise ValueError("color-mix percentages sum to zero")
    w1, w2 = p1 / total, p2 / total
    if space == "oklch":
        if c2[3] != 0.0 and c1[3] != 0.0:
            raise ValueError("oklch mix of two visible colours is not implemented")
        visible, weight = (c1, w1) if c2[3] == 0.0 else (c2, w2)
        return (visible[0], visible[1], visible[2], visible[3] * weight)
    if space != "srgb":
        raise ValueError(f"unsupported color-mix space: {space}")
    a = c1[3] * w1 + c2[3] * w2
    if a == 0.0:
        return (0.0, 0.0, 0.0, 0.0)
    out = [(c1[i] * c1[3] * w1 + c2[i] * c2[3] * w2) / a for i in range(3)]
    return (out[0], out[1], out[2], a)


# ---------------------------------------------------------------------------
# theme.css parsing.
# ---------------------------------------------------------------------------

DECL = re.compile(r"^\s*(--[A-Za-z0-9_-]+)\s*:\s*(.+?);\s*$")

# Vars the *application* writes onto `:root` at runtime (the Settings font
# pickers), never declared in any stylesheet. A `var()` on one of these with no
# fallback has no static value at all, which is recorded as an empty stack
# rather than guessed at.
RUNTIME_VARS = {"--app-font-family", "--editor-font-family"}


def strip_comments(text: str) -> str:
    return re.sub(r"/\*.*?\*/", "", text, flags=re.S)


def parse_blocks(path: Path):
    """Return (order, theme_inline, root, dark) from theme.css."""
    blocks = {"@theme inline": {}, ":root": {}, ".dark": {}}
    order = []
    cur = None
    decl_lines = 0
    for line in strip_comments(path.read_text()).split("\n"):
        m = re.match(r"^\s*(@theme inline|:root|\.dark)\s*\{", line)
        if m:
            cur = m.group(1)
        d = DECL.match(line)
        if d and cur is not None:
            name, value = d.group(1), d.group(2).strip()
            decl_lines += 1
            blocks[cur][name] = value
            if name not in order:
                order.append(name)
    if decl_lines != EXPECTED_DECL_LINES:
        raise SystemExit(
            f"theme.css has {decl_lines} declaration lines, expected "
            f"{EXPECTED_DECL_LINES} — the React tokens moved; re-measure before "
            "regenerating."
        )
    return order, blocks["@theme inline"], blocks[":root"], blocks[".dark"]


def parse_file_tree_extras(path: Path):
    """The three tokens the file tree declares in its own stylesheet."""
    wanted = ("--file-tree-hover-bg", "--file-tree-guide-icon-offset", "--tree-guide-color")
    found = {}
    for line in strip_comments(path.read_text()).split("\n"):
        d = DECL.match(line)
        if d and d.group(1) in wanted and d.group(1) not in found:
            found[d.group(1)] = d.group(2).strip()
    missing = [w for w in wanted if w not in found]
    if missing:
        raise SystemExit(f"file-explorer-tree.css is missing {missing}")
    return [(w, found[w]) for w in wanted]


# ---------------------------------------------------------------------------
# Resolution.
# ---------------------------------------------------------------------------


class Resolver:
    def __init__(self, env):
        self.env = env
        self.stack = []
        self.memo = {}

    def token(self, name: str) -> str:
        if name in self.memo:
            return self.memo[name]
        if name in self.stack:
            raise SystemExit(f"cyclic var chain: {' -> '.join(self.stack)} -> {name}")
        if name not in self.env:
            raise SystemExit(f"unresolved var: {name}")
        self.stack.append(name)
        out = self.expand(self.env[name])
        self.stack.pop()
        self.memo[name] = out
        return out

    def expand(self, value: str) -> str:
        """Substitute every `var()` in `value`, innermost-first, textually."""
        while True:
            idx = value.find("var(")
            if idx < 0:
                return value.strip()
            depth = 0
            for end in range(idx, len(value)):
                if value[end] == "(":
                    depth += 1
                elif value[end] == ")":
                    depth -= 1
                    if depth == 0:
                        break
            else:
                raise SystemExit(f"unbalanced var(): {value!r}")
            inner = value[idx + 4 : end]
            parts = split_top_level(inner)
            name = parts[0].strip()
            if name in self.env:
                sub = self.token(name)
            elif len(parts) > 1:
                sub = self.expand(", ".join(parts[1:]))
            elif name in RUNTIME_VARS:
                sub = ""
            else:
                raise SystemExit(f"unresolved var: {name}")
            value = value[:idx] + sub + value[end + 1 :]


def eval_color(expr: str):
    """Evaluate a fully `var()`-expanded colour expression."""
    expr = expr.strip()
    m = re.fullmatch(r"color-mix\(\s*(.+?)\s*\)", expr, re.S)
    if m:
        args = split_top_level(m.group(1))
        if len(args) != 3:
            raise ValueError(f"color-mix needs 3 arguments: {expr!r}")
        space = args[0].split()[-1]
        c1, p1 = split_pct(args[1])
        c2, p2 = split_pct(args[2])
        if p1 is None and p2 is None:
            p1 = p2 = 50.0
        elif p1 is None:
            p1 = 100.0 - p2
        elif p2 is None:
            p2 = 100.0 - p1
        return color_mix(space, eval_color(c1), p1, eval_color(c2), p2)
    return parse_color(expr)


def split_pct(arg: str):
    m = re.fullmatch(rf"(.*?)\s+({NUM})%", arg.strip(), re.S)
    if m:
        return m.group(1).strip(), float(m.group(2))
    return arg.strip(), None


def eval_length(expr: str) -> float:
    """Evaluate a length expression to pixels (1rem == 16px, the browser root)."""
    expr = expr.strip()
    m = re.fullmatch(r"calc\(\s*(.+?)\s*\)", expr, re.S)
    if m:
        body = m.group(1)
        factors = split_top_level(body, "*")
        if len(factors) < 2:
            raise ValueError(f"only multiplicative calc() is supported: {expr!r}")
        out = 1.0
        seen_length = False
        for f in factors:
            f = f.strip()
            if re.fullmatch(rf"{NUM}(px|rem)", f):
                out *= eval_length(f)
                seen_length = True
            else:
                out *= float(f)
        if not seen_length:
            raise ValueError(f"calc() with no length: {expr!r}")
        return out
    m = re.fullmatch(rf"({NUM})px", expr)
    if m:
        return float(m.group(1))
    m = re.fullmatch(rf"({NUM})rem", expr)
    if m:
        return float(m.group(1)) * 16.0
    raise ValueError(f"not a length: {expr!r}")


def eval_duration_ms(expr: str) -> float:
    """Pull the duration out of an `animation` shorthand."""
    for tok in expr.replace(",", " ").split():
        m = re.fullmatch(rf"({NUM})ms", tok)
        if m:
            return float(m.group(1))
        m = re.fullmatch(rf"({NUM})s", tok)
        if m and float(m.group(1)) >= 0:
            return float(m.group(1)) * 1000.0
    raise ValueError(f"no duration in {expr!r}")


def eval_font_stack(expr: str):
    out = []
    for part in split_top_level(expr):
        part = part.strip().strip("'\"")
        if part:
            out.append(part)
    return out


# ---------------------------------------------------------------------------
# Token classification.
# ---------------------------------------------------------------------------


def classify(name: str) -> str:
    if name.startswith("--animate-"):
        return "Duration"
    if name in ("--font-sans", "--font-heading", "--font-mono", "--font-editor"):
        return "FontFamily"
    if name == "--app-ui-scale":
        return "Scale"
    if name == "--radius" or name.startswith("--radius-") or name == "--app-scrollbar-radius":
        return "Radius"
    if name.startswith("--ui-text-"):
        return "FontSize"
    if name in (
        "--app-scrollbar-size",
        "--app-scrollbar-thumb-border",
        "--file-tree-guide-icon-offset",
    ):
        return "Space"
    return "Color"


def field_name(css: str) -> str:
    return css[2:].replace("-", "_")


def as_f32(x: float) -> float:
    return struct.unpack("<f", struct.pack("<f", x))[0]


def rust_f32(x: float) -> str:
    """The shortest decimal that round-trips as `f32`, with digit separators.

    Both halves are clippy's doing and both are worth having: an `f64`-precision
    literal assigned to an `f32` field is `excessive_precision` (it claims a
    precision the type cannot hold), and a run of eight digits with no `_` is
    `unreadable_literal`.
    """
    target = as_f32(x)
    text = repr(target)
    for digits in range(1, 18):
        candidate = f"{target:.{digits}g}"
        # `%g` reaches for scientific notation on round numbers (`10` becomes
        # `1e+01`); every token here is a plain decimal, so skip those forms.
        if "e" in candidate or "E" in candidate:
            continue
        if as_f32(float(candidate)) == target:
            text = candidate
            break
    if "e" in text or "E" in text:
        raise ValueError(f"unexpected exponent form: {text}")
    whole, _, frac = text.partition(".")
    frac = frac or "0"
    if len(whole) > 4:
        whole = "_".join(
            reversed([whole[max(0, i - 3) : i] for i in range(len(whole), 0, -3)])
        )
    if len(frac) > 5:
        frac = "_".join(frac[i : i + 3] for i in range(0, len(frac), 3))
    return f"{whole}.{frac}"


# `--app-scrollbar-thumb-border: 3px solid transparent` is a border shorthand.
# Only the width survives the port: `solid transparent` is the CSS trick that
# insets a scrollbar thumb via `background-clip`, and gpui has no analogue —
# the inset is drawn directly. The token therefore carries the 3px and nothing
# else, which is stated rather than silently assumed.
LEADING_LENGTH_ONLY = {"--app-scrollbar-thumb-border"}


def emit_value(name: str, kind: str, resolved: str):
    """Render one token's value as a Rust `seal(...)` expression."""
    if name in LEADING_LENGTH_ONLY:
        resolved = resolved.split()[0]
    if kind == "Color":
        r, g, b, a = eval_color(resolved)
        h, s, lightness = srgb_to_hsl(r, g, b)
        return (
            "Color::seal(Hsla { "
            f"h: {rust_f32(h)}, s: {rust_f32(s)}, l: {rust_f32(lightness)}, "
            f"a: {rust_f32(a)} }})"
        )
    if kind in ("Space", "Radius"):
        return f"{kind}::seal(px({rust_f32(eval_length(resolved))}))"
    if kind == "FontSize":
        return f"FontSize::seal(Rems({rust_f32(eval_length(resolved) / 16.0)}))"
    if kind == "Duration":
        ms = eval_duration_ms(resolved)
        if ms != int(ms):
            raise ValueError(f"non-integral millisecond duration: {resolved!r}")
        ms = int(ms)
        # `clippy::unnecessary_duration_from_millis`: a whole number of seconds
        # has to say so.
        if ms % 1000 == 0:
            return f"Duration::seal(StdDuration::from_secs({ms // 1000}))"
        return f"Duration::seal(StdDuration::from_millis({ms}))"
    if kind == "FontFamily":
        stack = eval_font_stack(resolved)
        inner = ", ".join(f'"{f}"' for f in stack)
        return f"FontFamily::seal(&[{inner}])"
    if kind == "Scale":
        return f"Scale::seal({rust_f32(float(resolved))})"
    raise ValueError(kind)


HEADER = '''//! The Crowbar design tokens, resolved from the React app's `theme.css`.
//!
//! **Generated file — do not edit.** Regenerate with
//! `python3 crates/crowbar-ui/tools/gen-theme.py` after changing
//! `web/src/styles/theme.css`.
//!
//! The React app carries {distinct} distinct token names across {decls}
//! declaration lines: {dual} are declared in both `:root` and `.dark` and so
//! carry a light *and* a dark value, and {single} are declared once and are
//! theme-invariant. That is why [`Theme`] is {total} fields and two `const`
//! tables rather than {flat} flat fields — the light/dark split is in the
//! tables, not in the field list. The last {extra} fields come from the file
//! tree's own stylesheet, which declares tokens the first components need.
//!
//! Every value here was resolved the way a browser resolves it: `var()` chains
//! followed to their source, `oklch()` converted through `OKLab` to sRGB and then
//! to gpui's `Hsla`, `color-mix()` evaluated with premultiplied alpha, `calc()`
//! folded, and `rem` taken at the browser root of 16px.

use gpui::{{Hsla, Rems, px}};
use std::time::Duration as StdDuration;

use super::token::{{Color, Duration, FontFamily, FontSize, Radius, Scale, Space}};

/// Every Crowbar design token, resolved for one appearance.
///
/// Construct nothing: use [`Theme::LIGHT`], [`Theme::DARK`] or
/// [`Theme::for_appearance`]. The fields are sealed newtypes, so a view crate
/// can read a token and cannot mint one.
#[derive(Clone, Debug, PartialEq)]
pub struct Theme {{
'''


def main() -> None:
    check_palette()

    order, theme_inline, root, dark = parse_blocks(THEME_CSS)
    extras = parse_file_tree_extras(FILE_TREE_CSS)

    distinct = set(theme_inline) | set(root) | set(dark)
    dual = set(root) & set(dark)
    if len(distinct) != EXPECTED_DISTINCT or len(dual) != EXPECTED_DUAL:
        raise SystemExit(
            f"theme.css measures {len(distinct)} distinct tokens / {len(dual)} "
            f"dual, expected {EXPECTED_DISTINCT}/{EXPECTED_DUAL}"
        )

    light_env = dict(TAILWIND_SRC)
    light_env.update(root)
    light_env.update(theme_inline)
    dark_env = dict(TAILWIND_SRC)
    dark_env.update(root)
    dark_env.update(dark)
    dark_env.update(theme_inline)

    extra_names = [n for n, _ in extras]
    for name, value in extras:
        light_env[name] = value
        dark_env[name] = value

    light = Resolver(light_env)
    darkr = Resolver(dark_env)

    names = order + extra_names
    if len(names) != EXPECTED_DISTINCT + len(extras):
        raise SystemExit(f"emitting {len(names)} tokens, expected {EXPECTED_DISTINCT + len(extras)}")

    fields, light_rows, dark_rows, diff_rows = [], [], [], []
    varying = 0
    dual_varying = 0
    for name in names:
        kind = classify(name)
        try:
            lv = emit_value(name, kind, light.token(name))
            dv = emit_value(name, kind, darkr.token(name))
        except Exception as exc:  # noqa: BLE001 — a bad token must be loud
            raise SystemExit(f"{name}: {exc}") from exc
        fields.append(
            f"    /// `{name}`: `{light_env[name]}`\n"
            f"    pub {field_name(name)}: {kind},\n"
        )
        light_rows.append(f"        {field_name(name)}: {lv},\n")
        dark_rows.append(f"        {field_name(name)}: {dv},\n")
        diff_rows.append(
            f'    ("{name}", |light, dark| '
            f"light.{field_name(name)} != dark.{field_name(name)}),\n"
        )
        if lv != dv:
            varying += 1
            if name in dual:
                dual_varying += 1

    out = [
        HEADER.format(
            distinct=EXPECTED_DISTINCT,
            decls=EXPECTED_DECL_LINES,
            dual=EXPECTED_DUAL,
            single=EXPECTED_DISTINCT - EXPECTED_DUAL,
            total=len(names),
            flat=EXPECTED_DECL_LINES,
            extra=len(extras),
        )
    ]
    out.extend(fields)
    out.append("}\n\n")
    out.append("impl Theme {\n")
    out.append("    /// The light appearance — `theme.css`'s `:root` block.\n")
    out.append("    pub const LIGHT: Self = Self {\n")
    out.extend(light_rows)
    out.append("    };\n\n")
    out.append("    /// The dark appearance — `:root` with `.dark` layered over it.\n")
    out.append("    pub const DARK: Self = Self {\n")
    out.extend(dark_rows)
    out.append("    };\n}\n")
    out.append(
        "\n/// Every token, paired with a predicate that asks whether the two\n"
        "/// tables disagree about it.\n"
        "///\n"
        "/// Generated alongside the tables from the same field list, so the\n"
        "/// structural tests cannot drift out of step with what was emitted.\n"
        "#[cfg(test)]\n"
        "pub(super) type VarianceRow = (&'static str, fn(&Theme, &Theme) -> bool);\n"
        "\n#[cfg(test)]\n"
        f"pub(super) const TOKEN_VARIANCE: [VarianceRow; {len(names)}] = [\n"
    )
    out.extend(diff_rows)
    out.append("];\n")
    out.append(
        "\n/// The token names `theme.css` declares in **both** `:root` and `.dark`.\n"
        "///\n"
        "/// Ten of them declare the *same* value in both blocks (`--primary` and\n"
        "/// friends), so being on this list is not the same as varying by\n"
        "/// appearance; and sixty tokens that are declared once still vary,\n"
        "/// because they alias one that does. `token_variance` is the truth about\n"
        "/// what actually differs — this is the truth about what the CSS declares.\n"
        "#[cfg(test)]\n"
        f"pub(super) const DUAL_DECLARED: [&str; {len(dual)}] = [\n"
    )
    for name in sorted(dual):
        out.append(f'    "{name}",\n')
    out.append("];\n")

    OUT.write_text("".join(out))
    # Canonicalise, so that `cargo fmt --check` stays green across a
    # regeneration and a diff of this file only ever shows changed values.
    subprocess.run(
        ["rustfmt", "--edition", "2024", str(OUT)], check=True, capture_output=True
    )
    print(f"wrote {OUT.relative_to(REPO)}")
    print(f"  tokens emitted:        {len(names)} ({EXPECTED_DISTINCT} from theme.css + {len(extras)} extras)")
    print(f"  dual-declared:         {len(dual)} ({dual_varying} of them actually differ)")
    print(f"  light/dark disagree:   {varying} ({varying - dual_varying} are single-declared aliases of one that does)")
    kinds = {}
    for name in names:
        kinds[classify(name)] = kinds.get(classify(name), 0) + 1
    for k in sorted(kinds):
        print(f"  {k:<12} {kinds[k]}")


TAILWIND_SRC = {k: v[0] for k, v in TAILWIND.items()}

if __name__ == "__main__":
    import sys as _sys

    if "--check-vendored" in _sys.argv:
        raise SystemExit(check_vendored())
    sys.exit(main())
