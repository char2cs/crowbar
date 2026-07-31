//! `--surface spinner`: what taffy resolves a rotating glyph to, in a real
//! window.
//!
//! **These numbers are the port's, and they are the reference's *at rest*.** The
//! reference's `bounds` are only 16 × 16 at four instants per second — see
//! `crowbar_ui::components::spinner` for the measured excursion — so what is
//! pinned here is the layout box, which is what the port paints and what a
//! capture taken at `currentTime = 0` records.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::components::spinner;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "spinner"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the glyph itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, spinner::ID_SPINNER, id)
}

/// The default cell is `loading-spinner.tsx`'s `size-4`: 16 × 16, the box the
/// reference reports, with no paint of any kind on it.
#[gpui::test]
fn the_default_cell_is_the_captured_sixteen_pixel_glyph(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec!["spinner".to_owned()],
        "the host row carries no anchor",
    );

    let root = at(&records, "spinner");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(16.0));
    assert_px(root.size.height, px(16.0));

    let record = find(&records, "spinner");
    assert_px(record.radius, px(0.0));
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    assert_eq!(
        record.background,
        Paint::None,
        "an <svg> paints no background"
    );
    assert!(
        record.text.is_none(),
        "the glyph has no text node, so the reference emits no fg and no font either",
    );
}

/// Every call site's box, and the one that moves at 640px.
///
/// The control is the other three: a port that read the breakpoint everywhere
/// would be wrong three times over, and this asserts each of them is unmoved.
#[gpui::test]
fn only_the_button_indicator_changes_across_the_breakpoint(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for (site, expected) in [
        ("none", px(24.0)),
        ("loading-spinner", px(16.0)),
        ("loading-spinner-compact", px(12.0)),
    ] {
        for viewport in ["639", "1714"] {
            let records = measure(
                cx,
                cell(&["--call-site", site, "--viewport-width", viewport]),
            );
            let box_ = at(&records, "spinner");
            assert_px(box_.size.width, expected);
            assert_px(box_.size.height, expected);
        }
    }

    let narrow = measure(
        cx,
        cell(&[
            "--call-site",
            "button-loading-indicator",
            "--viewport-width",
            "639",
        ]),
    );
    let wide = measure(
        cx,
        cell(&[
            "--call-site",
            "button-loading-indicator",
            "--viewport-width",
            "1714",
        ]),
    );
    assert_px(at(&narrow, "spinner").size.width, px(18.0));
    assert_px(at(&wide, "spinner").size.width, px(16.0));
}

/// §8.3's `empty`: `size={0}` gives a **zero-area** box, which paints nothing —
/// `skeleton`'s cell, reached from the props. It overrides a call site that
/// would have pinned a box.
#[gpui::test]
fn the_empty_cell_has_no_area_and_overrides_every_call_site(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for site in ["none", "loading-spinner", "button-loading-indicator"] {
        let records = measure(cx, cell(&["--flags", "empty", "--call-site", site]));
        let root = at(&records, "spinner");
        assert_px(root.size.width, px(0.0));
        assert_px(root.size.height, px(0.0));
        assert!(
            !find(&records, "spinner").visible,
            "a zero-area box paints nothing: {site}",
        );
    }
}
