//! Comparing the Rust sidebar against the Tauri app's, pixel for pixel.
//!
//! # Why this exists
//!
//! Every earlier attempt at "do these look the same" was one of:
//!
//! * an anchor diff — blind to artwork, and to anything unanchored;
//! * a screenshot of Crowbar-**React in a browser**, which has no vibrancy and
//!   therefore paints a ground the real app never shows;
//! * my own eye on two images, which was wrong repeatedly.
//!
//! The reference has to be the **Tauri** app, captured through its own MCP
//! bridge, and the comparison has to be arithmetic.
//!
//! # What makes the two comparable
//!
//! Both captures are DPR 2, and both sidebars are 294 logical (588 physical)
//! pixels wide, so the sidebar occupies the same physical columns in each.
//! The reference is cropped to those columns and the two are compared row by
//! row.
//!
//! # What it deliberately does not claim
//!
//! The Tauri capture composites over the desktop through the window's
//! vibrancy; the headless render has no desktop behind it. So the **ground**
//! differs by construction and a raw whole-image difference would be
//! dominated by it. What is compared is what the app itself paints: the
//! opaque rows, the text, and the glyphs — reported per region so a
//! difference names the thing that differs rather than a single number.

use std::path::Path;

use image::RgbaImage;

/// One region of the sidebar, in **logical** pixels from its top-left.
#[derive(Debug, Clone, Copy)]
pub struct Region {
    /// What this region is, for the report.
    pub name: &'static str,
    /// Left edge.
    pub x: u32,
    /// Top edge.
    pub y: u32,
    /// Width.
    pub w: u32,
    /// Height.
    pub h: u32,
}

/// What one region's comparison found.
#[derive(Debug, Clone, Copy)]
pub struct RegionReport {
    /// The region compared.
    pub region: Region,
    /// Mean colour of the reference, `[r, g, b]`.
    pub reference: [u8; 3],
    /// Mean colour of the Rust render.
    pub mine: [u8; 3],
    /// Largest per-channel difference of the means.
    pub delta: u8,
}

/// Mean colour of `region` in `image`, at `scale` device pixels per logical.
fn mean(image: &RgbaImage, region: Region, scale: u32, x_offset: u32) -> [u8; 3] {
    let (mut red, mut green, mut blue, mut count) = (0u64, 0u64, 0u64, 0u64);
    for y in (region.y * scale)..((region.y + region.h) * scale) {
        for x in ((region.x * scale) + x_offset)..(((region.x + region.w) * scale) + x_offset) {
            if x >= image.width() || y >= image.height() {
                continue;
            }
            let pixel = image.get_pixel(x, y).0;
            red += u64::from(pixel[0]);
            green += u64::from(pixel[1]);
            blue += u64::from(pixel[2]);
            count += 1;
        }
    }
    if count == 0 {
        return [0, 0, 0];
    }
    // A mean of `u8` channels cannot exceed 255, so each `try_into` holds;
    // `unwrap_or(255)` states that rather than leaning on it.
    [
        u8::try_from(red / count).unwrap_or(u8::MAX),
        u8::try_from(green / count).unwrap_or(u8::MAX),
        u8::try_from(blue / count).unwrap_or(u8::MAX),
    ]
}

/// Compare `mine` against `reference` over `regions`.
///
/// # Panics
///
/// Test-only instrument; it panics rather than threading an error nobody
/// would handle.
#[must_use]
pub fn compare(reference: &Path, mine: &Path, regions: &[Region]) -> Vec<RegionReport> {
    let reference = image::open(reference)
        .expect("the reference png opens")
        .to_rgba8();
    let mine_img = image::open(mine).expect("the render png opens").to_rgba8();
    let scale = 2; // both captures are DPR 2

    regions
        .iter()
        .map(|region| {
            let a = mean(&reference, *region, scale, 0);
            let b = mean(&mine_img, *region, scale, 0);
            let delta = (0..3).map(|i| a[i].abs_diff(b[i])).max().unwrap_or(0);
            RegionReport {
                region: *region,
                reference: a,
                mine: b,
                delta,
            }
        })
        .collect()
}

/// The regions worth comparing, in logical pixels from the sidebar's top-left.
///
/// Chosen to be things the app paints rather than the ground it paints on:
/// the opaque project row, a repo header, a workspace row, and the tab strip.
#[must_use]
pub const fn sidebar_regions() -> [Region; 7] {
    [
        Region {
            name: "header",
            x: 0,
            y: 0,
            w: 294,
            h: 44,
        },
        Region {
            name: "context-pill",
            x: 8,
            y: 44,
            w: 278,
            h: 48,
        },
        Region {
            name: "tab-strip",
            x: 8,
            y: 101,
            w: 278,
            h: 36,
        },
        Region {
            name: "project-row",
            x: 6,
            y: 145,
            w: 282,
            h: 36,
        },
        Region {
            name: "repo-row",
            x: 6,
            y: 185,
            w: 282,
            h: 36,
        },
        Region {
            name: "workspace-row",
            x: 20,
            y: 225,
            w: 268,
            h: 36,
        },
        // Bare ground, below every row: what the window paints when the app
        // itself paints nothing. The difference here IS the backdrop, and
        // every semi-transparent region above inherits it.
        Region {
            name: "bare-ground",
            x: 10,
            y: 400,
            w: 274,
            h: 120,
        },
    ]
}

/// How many **logical** rows the contact sheet covers. The sidebar's content
/// is top-anchored and the rest is bare ground, so 400 covers every row this
/// slice draws while keeping the sheet legible when a viewer downscales it.
const SHEET_ROWS: u32 = 400;

/// The glyphs worth magnifying: the two branch marks, the repo avatar, one
/// label, and the active row's own border. Between them they cover every
/// class of artwork this slice paints.
const ZOOMS: &[Region] = &[
    Region {
        name: "repo-avatar",
        x: 8,
        y: 188,
        w: 30,
        h: 30,
    },
    Region {
        name: "repo-avatar-2",
        x: 8,
        y: 266,
        w: 30,
        h: 30,
    },
    Region {
        name: "pill-left",
        x: 12,
        y: 48,
        w: 120,
        h: 44,
    },
    Region {
        name: "pill-logo",
        x: 240,
        y: 50,
        w: 48,
        h: 40,
    },
    Region {
        name: "home-row-icon",
        x: 8,
        y: 148,
        w: 34,
        h: 30,
    },
    Region {
        name: "lock",
        x: 24,
        y: 230,
        w: 22,
        h: 22,
    },
    Region {
        name: "warning",
        x: 24,
        y: 308,
        w: 22,
        h: 22,
    },
    Region {
        name: "tab-glyphs",
        x: 8,
        y: 101,
        w: 200,
        h: 36,
    },
];

/// Everything [`diff_pixels`] writes to disk, and the images it needs to do
/// it. A struct rather than eight parameters, and split out of `diff_pixels`
/// so that function stays about the arithmetic.
struct Artefacts<'a> {
    reference: &'a RgbaImage,
    mine: &'a RgbaImage,
    /// The raw per-pixel delta map.
    map: &'a RgbaImage,
    width: u32,
    height: u32,
    scale: u32,
    heatmap: &'a Path,
}

/// Write the heat maps, the compared crop, the contact sheet and the zooms.
///
/// # Panics
///
/// Test-only instrument; it panics rather than threading an error nobody would
/// handle.
fn write_artefacts(art: &Artefacts) {
    let Artefacts {
        reference,
        mine,
        map,
        width,
        height,
        scale,
        heatmap,
    } = *art;
    // The heat map is written twice: once raw, and once amplified 4x and
    // clamped. Raw is the honest record; amplified is the one a human can
    // actually see, because a real glyph difference is a few hundred pixels
    // out of 1.3 million and vanishes when the viewer downscales the image.
    map.save(heatmap).expect("the heat map is written");
    let mut loud = RgbaImage::new(width, height);
    for (x, y, pixel) in map.enumerate_pixels() {
        let value = pixel.0[0].saturating_mul(4);
        loud.put_pixel(x, y, image::Rgba([value, value, value, 255]));
    }
    loud.save(heatmap.with_extension("loud.png"))
        .expect("the amplified heat map is written");

    // And the two crops that were actually compared, so "what did it look at"
    // is never again a question answered by cropping the images by hand — the
    // centre-crop that mistake produced compared this sidebar against the
    // reference's *content pane* and reported a plausible-looking p50 of 0.
    let mut left = RgbaImage::new(width, height);
    for y in 0..height {
        for x in 0..width {
            left.put_pixel(x, y, *reference.get_pixel(x, y));
        }
    }
    left.save(heatmap.with_file_name("compared-reference.png"))
        .expect("the compared reference crop is written");

    // A contact sheet: reference | render | amplified difference, side by
    // side over the same rows, with a one-pixel rule between them. This is
    // the artefact a human looks at, and putting the three panels in one
    // image is what makes "are these the same" a glance instead of three
    // separate crops that can silently disagree about their offsets.
    let sheet_rows = height.min(SHEET_ROWS * scale);
    let gap = 4;
    let mut sheet = RgbaImage::new(width * 3 + gap * 2, sheet_rows);
    for y in 0..sheet_rows {
        for x in 0..width {
            sheet.put_pixel(x, y, *reference.get_pixel(x, y));
            sheet.put_pixel(x + width + gap, y, *mine.get_pixel(x, y));
            sheet.put_pixel(x + (width + gap) * 2, y, *loud.get_pixel(x, y));
        }
        for rule in 0..gap {
            let ink = image::Rgba([255, 0, 0, 255]);
            sheet.put_pixel(width + rule, y, ink);
            sheet.put_pixel((width * 2) + gap + rule, y, ink);
        }
    }
    sheet
        .save(heatmap.with_file_name("contact-sheet.png"))
        .expect("the contact sheet is written");

    // Zoom panels. A 16x16 glyph is 32 device pixels; at that size the eye
    // cannot tell "antialiased differently" from "drawn wrong", and those are
    // the two hypotheses that matter. Nearest-neighbour magnification keeps
    // the pixel grid visible, so an edge-only difference reads as an outline
    // and a displaced or substituted glyph reads as a solid mass.
    for zoom in ZOOMS {
        let factor = 6;
        let (zw, zh) = (zoom.w * scale, zoom.h * scale);
        let mut panel = RgbaImage::new((zw * factor * 3) + 8, zh * factor);
        for y in 0..zh {
            for x in 0..zw {
                let (sx, sy) = ((zoom.x * scale) + x, (zoom.y * scale) + y);
                if sx >= width || sy >= height {
                    continue;
                }
                let trio = [
                    *reference.get_pixel(sx, sy),
                    *mine.get_pixel(sx, sy),
                    *loud.get_pixel(sx, sy),
                ];
                for (panel_index, ink) in trio.into_iter().enumerate() {
                    let origin = (zw * factor * u32::try_from(panel_index).unwrap_or(0))
                        + (4 * u32::try_from(panel_index.min(2)).unwrap_or(0));
                    for dy in 0..factor {
                        for dx in 0..factor {
                            let (px, py) = (origin + (x * factor) + dx, (y * factor) + dy);
                            if px < panel.width() && py < panel.height() {
                                panel.put_pixel(px, py, ink);
                            }
                        }
                    }
                }
            }
        }
        panel
            .save(heatmap.with_file_name(format!("zoom-{}.png", zoom.name)))
            .expect("the zoom panel is written");
    }
}

/// One device pixel from each image, at a **logical** coordinate.
///
/// A region mean answers "are these the same on average", which is the
/// question that let a lighter, hovered reference row read as a match. This
/// answers "what colour is this exact pixel", which is the one that catches it.
#[must_use]
pub fn probe(reference: &Path, mine: &Path, x: u32, y: u32, scale: u32) -> ([u8; 3], [u8; 3]) {
    let reference = image::open(reference)
        .expect("the reference png opens")
        .to_rgba8();
    let mine = image::open(mine).expect("the render png opens").to_rgba8();
    let at = |image: &RgbaImage| -> [u8; 3] {
        let pixel = image
            .get_pixel(
                (x * scale).min(image.width() - 1),
                (y * scale).min(image.height() - 1),
            )
            .0;
        [pixel[0], pixel[1], pixel[2]]
    };
    (at(&reference), at(&mine))
}

/// A whole-image comparison, and a picture of where the two differ.
///
/// # Why a per-pixel pass, when [`compare`] already reports per region
///
/// A region mean is blind in exactly the way that matters here: an icon drawn
/// with the wrong glyph, or not drawn at all, moves a 36x36 row's mean by
/// about a level. Every confident-and-wrong "it matches" this session produced
/// came from a summary statistic. This reports the **worst single pixel**, and
/// writes a heat map so the difference has a location rather than a number.
///
/// # The ground offset is reported, not hidden
///
/// The reference composites over the desktop through vibrancy and the headless
/// render stands a flat neutral in for it, so bare ground differs by
/// construction. That shows up here as a small, uniform delta across the whole
/// image — which is why `p50` is reported next to `max`. A uniform p50 with a
/// small max is the signature of "same painting, slightly different ground";
/// a small p50 with a large localised max is the signature of a real defect,
/// and the heat map says where.
#[derive(Debug, Clone)]
pub struct PixelDiff {
    /// Device pixels compared.
    pub compared: u64,
    /// Median per-pixel delta — the ground offset, in practice.
    pub p50: u8,
    /// 99.9th percentile per-pixel delta.
    pub p999: u8,
    /// Largest per-pixel delta anywhere.
    pub max: u8,
    /// Where `max` was found, in **logical** pixels.
    pub max_at: (u32, u32),
    /// Per **logical** row: the row's worst delta. Indexed from the top.
    pub row_max: Vec<u8>,
}

/// Compare every pixel of `mine` against the same columns of `reference`, and
/// write a heat map to `heatmap` (black = identical, white = maximally
/// different).
///
/// # Panics
///
/// Test-only instrument; it panics rather than threading an error nobody would
/// handle.
#[must_use]
pub fn diff_pixels(reference: &Path, mine: &Path, scale: u32, heatmap: &Path) -> PixelDiff {
    let reference = image::open(reference)
        .expect("the reference png opens")
        .to_rgba8();
    let mine = image::open(mine).expect("the render png opens").to_rgba8();

    let width = reference.width().min(mine.width());
    let height = reference.height().min(mine.height());

    let mut deltas: Vec<u8> = Vec::with_capacity((width as usize) * (height as usize));
    let mut map = RgbaImage::new(width, height);
    let mut row_max = vec![0u8; (height / scale) as usize + 1];
    let (mut max, mut max_at) = (0u8, (0u32, 0u32));

    for y in 0..height {
        for x in 0..width {
            let (a, b) = (reference.get_pixel(x, y).0, mine.get_pixel(x, y).0);
            // Alpha is deliberately excluded: the reference is an opaque
            // capture and the render is opaque too, but the *reason* they are
            // opaque differs, and a difference in alpha nobody can see is not
            // a difference in how the app looks.
            let delta = (0..3)
                .map(|channel| a[channel].abs_diff(b[channel]))
                .max()
                .unwrap_or(0);
            deltas.push(delta);
            map.put_pixel(x, y, image::Rgba([delta, delta, delta, 255]));
            if delta > max {
                max = delta;
                max_at = (x / scale, y / scale);
            }
            let row = (y / scale) as usize;
            if let Some(slot) = row_max.get_mut(row)
                && delta > *slot
            {
                *slot = delta;
            }
        }
    }

    write_artefacts(&Artefacts {
        reference: &reference,
        mine: &mine,
        map: &map,
        width,
        height,
        scale,
        heatmap,
    });

    deltas.sort_unstable();

    PixelDiff {
        compared: deltas.len() as u64,
        p50: percentile(&deltas, 500),
        p999: percentile(&deltas, 999),
        max,
        max_at,
        row_max,
    }
}

/// The value at `per_mille` of a **sorted** slice.
///
/// Integer arithmetic throughout: a float index over a slice length is three
/// separate lossy casts, and the quantity being indexed is a count.
#[must_use]
fn percentile(sorted: &[u8], per_mille: usize) -> u8 {
    match sorted.len() {
        0 => 0,
        len => sorted
            .get(((len - 1) * per_mille) / 1000)
            .copied()
            .unwrap_or(0),
    }
}

#[cfg(test)]
mod tests {
    use super::{compare, sidebar_regions};

    /// Prints the per-region comparison of the Rust sidebar against the Tauri
    /// app's own capture.
    ///
    /// An instrument, not an assertion: it reports, and the numbers say which
    /// region differs and by how much.
    ///
    /// Both inputs have to exist. `target/tauri-sidebar.png` comes from the
    /// Tauri MCP bridge (`webview_screenshot`) with the window at 1536x1095
    /// and the sidebar at 294; `target/sidebar.png` comes from
    /// `write_the_sidebar_png`.
    #[test]
    #[ignore = "needs both captures on disk; see the doc comment"]
    fn report_the_sidebar_difference() {
        let dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../target");
        let reference = dir.join("tauri-sidebar.png");
        let mine = dir.join("sidebar.png");
        assert!(
            reference.exists(),
            "no tauri capture at {}",
            reference.display()
        );
        assert!(mine.exists(), "no render at {}", mine.display());

        println!(
            "{:<16} {:>18} {:>18} {:>7}",
            "region", "tauri", "rust", "delta"
        );
        for report in compare(&reference, &mine, &sidebar_regions()) {
            println!(
                "{:<16} {:>18} {:>18} {:>7}",
                report.region.name,
                format!(
                    "#{:02x}{:02x}{:02x}",
                    report.reference[0], report.reference[1], report.reference[2]
                ),
                format!(
                    "#{:02x}{:02x}{:02x}",
                    report.mine[0], report.mine[1], report.mine[2]
                ),
                report.delta,
            );
        }
    }

    /// Per-pixel, whole-sidebar difference against the Tauri capture, plus a
    /// heat map at `target/sidebar-diff.png`.
    ///
    /// An instrument, not an assertion — the same posture as
    /// `report_the_sidebar_difference`, and for the same reason: the numbers
    /// say whether the two are the same, and the heat map says where they are
    /// not.
    ///
    /// ```sh
    /// cargo test -p crowbar-app the_sidebar_pixel_difference -- --ignored --test-threads=1 --nocapture
    /// ```
    #[test]
    #[ignore = "needs both captures on disk; see the doc comment"]
    fn the_sidebar_pixel_difference() {
        let dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../target");
        let reference = dir.join("tauri-sidebar.png");
        let mine = dir.join("sidebar.png");
        assert!(
            reference.exists(),
            "no tauri capture at {}",
            reference.display()
        );
        assert!(mine.exists(), "no render at {}", mine.display());

        let diff = super::diff_pixels(&reference, &mine, 2, &dir.join("sidebar-diff.png"));
        println!(
            "compared={} p50={} p99.9={} max={} at logical {:?}",
            diff.compared, diff.p50, diff.p999, diff.max, diff.max_at
        );
        println!("--- probes: exact device pixels, reference vs render ---");
        for (name, x, y) in [
            ("bare ground", 150u32, 380u32),
            ("repo row bg", 150, 203),
            ("repo row bg 2", 150, 281),
            ("workspace row bg", 150, 241),
            ("home row bg", 150, 163),
            ("pill bg", 150, 70),
            ("tab list bg", 150, 119),
            ("active tab bg", 40, 119),
            ("header bg", 150, 20),
            ("inside zoom lock", 30, 236),
            ("inside zoom avatar", 12, 195),
            ("inside zoom avatar b", 34, 214),
            ("inside zoom pill", 20, 52),
        ] {
            let (a, b) = super::probe(&reference, &mine, x, y, 2);
            println!(
                "  {name:<18} ref #{:02x}{:02x}{:02x}  mine #{:02x}{:02x}{:02x}  delta {}",
                a[0],
                a[1],
                a[2],
                b[0],
                b[1],
                b[2],
                (0..3).map(|c| a[c].abs_diff(b[c])).max().unwrap_or(0)
            );
        }
        println!("--- logical rows whose worst pixel exceeds 24 ---");
        let mut run: Option<(usize, usize, u8)> = None;
        for (row, worst) in diff.row_max.iter().copied().enumerate() {
            match (&mut run, worst > 24) {
                (None, true) => run = Some((row, row, worst)),
                (Some(open), true) => {
                    open.1 = row;
                    open.2 = open.2.max(worst);
                }
                (Some(open), false) => {
                    println!("  y {:>4}..{:<4} worst={}", open.0, open.1, open.2);
                    run = None;
                }
                (None, false) => {}
            }
        }
        if let Some(open) = run {
            println!("  y {:>4}..{:<4} worst={}", open.0, open.1, open.2);
        }
    }
}
