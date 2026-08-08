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
}
