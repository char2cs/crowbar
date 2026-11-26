#!/usr/bin/env python3
"""Writes the static font instances the shell embeds, from the variable fonts they come from.

Usage: font-instances.py SRC_DIR OUT_DIR
SRC_DIR holds CalSansUI.ttf and GeistMono-Variable.ttf (the variable files develop's web app loads).
Stock GPUI draws a variable face at its default instance only, so every weight and the oblique the
shell sets is a face of its own. Both fonts are OFL 1.1 without a Reserved Font Name, so these
Modified Versions keep their family names and stay under the OFL.
Needs fontTools; run it in a throwaway container, never on the host:
  docker run --rm -v "$PWD:/f" python:3.12-slim sh -c \
    'pip install -q fonttools==4.60.1 && python -I /f/tools/scripts/font-instances.py /f/in /f/out'
"""
import math
import os
import sys

from fontTools.pens.recordingPen import DecomposingRecordingPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.ttLib import TTFont
from fontTools.varLib import instancer

# WebKit synthesizes italic as a 14 degree oblique; develop's provisional titles are drawn so.
OBLIQUE_DEGREES = 14.0
FS_SELECTION_ITALIC = 1 << 0
FS_SELECTION_REGULAR = 1 << 6
FS_SELECTION_OBLIQUE = 1 << 9
MAC_STYLE_BOLD = 1 << 0
MAC_STYLE_ITALIC = 1 << 1
HINTING_TABLES = ("fpgm", "prep", "cvt ", "hdmx", "LTSH", "VDMX")

# (source file, family, postscript stem, axes kept at their default, [(weight name, wght, oblique)])
FAMILIES = [
    ("CalSansUI.ttf", "Cal Sans UI", "CalSansUI", {"GEOM": 0}, [
        ("Regular", 400, False),
        ("Medium", 500, False),
        ("SemiBold", 600, False),
        ("Bold", 700, False),
        ("Medium", 500, True),
    ]),
    ("GeistMono-Variable.ttf", "Geist Mono", "GeistMono", {}, [
        ("Regular", 400, False),
        ("Bold", 700, False),
    ]),
]


def rename(font, family, stem, weight_name, oblique):
    name = font["name"]
    style = f"{weight_name} Oblique" if oblique else weight_name
    compact = style.replace(" ", "")
    ribbi = weight_name in ("Regular", "Bold")
    legacy_family = family if ribbi else f"{family} {weight_name}"
    legacy_style = ("Bold " if weight_name == "Bold" else "") + ("Italic" if oblique else "")
    legacy_style = legacy_style.strip() or "Regular"
    for name_id in (1, 2, 3, 4, 6, 16, 17, 25):
        name.removeNames(nameID=name_id)
    entries = {
        1: legacy_family,
        2: legacy_style,
        3: f"{stem}-{compact}",
        4: f"{family} {style}",
        6: f"{stem}-{compact}",
        16: family,
        17: style,
    }
    for name_id, value in entries.items():
        name.setName(value, name_id, 3, 1, 0x409)


def set_style_bits(font, weight, oblique):
    os2 = font["OS/2"]
    os2.usWeightClass = weight
    os2.fsSelection &= ~(FS_SELECTION_ITALIC | FS_SELECTION_REGULAR | FS_SELECTION_OBLIQUE)
    mac_style = MAC_STYLE_BOLD if weight >= 700 else 0
    if oblique:
        os2.fsSelection |= FS_SELECTION_ITALIC | FS_SELECTION_OBLIQUE
        mac_style |= MAC_STYLE_ITALIC
    if weight == 400 and not oblique:
        os2.fsSelection |= FS_SELECTION_REGULAR
    font["head"].macStyle = mac_style


def skew(font):
    slant = math.tan(math.radians(OBLIQUE_DEGREES))
    glyph_set = font.getGlyphSet()
    glyf = font["glyf"]
    outlines = {}
    for glyph_name in font.getGlyphOrder():
        recording = DecomposingRecordingPen(glyph_set)
        glyph_set[glyph_name].draw(recording)
        pen = TTGlyphPen(None)
        recording.replay(TransformPen(pen, (1, 0, slant, 1, 0, 0)))
        outlines[glyph_name] = pen.glyph()
    hmtx = font["hmtx"]
    for glyph_name, glyph in outlines.items():
        glyph.recalcBounds(glyf)
        glyf[glyph_name] = glyph
        advance, _ = hmtx[glyph_name]
        hmtx[glyph_name] = (advance, getattr(glyph, "xMin", 0))
    for tag in HINTING_TABLES:
        if tag in font:
            del font[tag]
    font["post"].italicAngle = -OBLIQUE_DEGREES
    font["hhea"].caretSlopeRise = 1000
    font["hhea"].caretSlopeRun = round(1000 * slant)


def write_instance(source, out_dir, family, stem, fixed_axes, weight_name, weight, oblique):
    font = instancer.instantiateVariableFont(TTFont(source), {"wght": weight, **fixed_axes})
    for tag in ("STAT", "MVAR", "HVAR"):
        if tag in font:
            del font[tag]
    rename(font, family, stem, weight_name, oblique)
    set_style_bits(font, weight, oblique)
    if oblique:
        skew(font)
    suffix = f"{weight_name}Oblique" if oblique else weight_name
    path = os.path.join(out_dir, f"{stem}-{suffix}.ttf")
    font.save(path)
    print(path)


def main(src_dir, out_dir):
    for file_name, family, stem, fixed_axes, styles in FAMILIES:
        source = os.path.join(src_dir, file_name)
        for weight_name, weight, oblique in styles:
            write_instance(source, out_dir, family, stem, fixed_axes, weight_name, weight, oblique)


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])
