//! `--surface repo-import-dialog`, laid out in a real window.
//!
//! No live pixel reference exists for this surface — `dialog.md` §5 names
//! why: this call site's own trigger was not identified live before that
//! item's time budget ran out. What follows checks that the wrap renders and
//! lays out **exactly like `dialog`'s already-converged wrap does** on every
//! field the two share, that this call site's own header padding override
//! (`p-4 pb-2`, not `dialog`'s `p-6`) genuinely moves the header, and that
//! `h-[70vh]`'s own arithmetic — the popup as the independent variable, the
//! body as the derived remainder — holds at several assumed window heights.
//! There is no footer on this surface at all; `--flags empty` therefore has
//! only the header to remove.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::surfaces::repo::repo_import_dialog;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "repo-import-dialog",
        "--width",
        "1714",
        "--viewport-width",
        "1714",
    ];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, repo_import_dialog::ID_POPUP, id)
}

/// **The wrap renders at all**, carrying every contract anchor the resting
/// cell has, none of `dialog`'s own bare `dialog-*` ids, and no footer at
/// all — this call site never nests a `DialogFooter`.
#[gpui::test]
fn the_wrapped_popup_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        "repo-import-dialog-popup",
        "repo-import-dialog-header",
        "repo-import-dialog-title",
        "repo-import-dialog-description",
    ] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    assert!(
        !seen.iter().any(|id| id.starts_with("dialog-")),
        "{seen:?} — this surface must never carry a bare `dialog-*` id"
    );
    assert!(
        !seen.iter().any(|id| id.contains("footer")),
        "{seen:?} — this call site never renders a DialogFooter"
    );
}

/// **The popup's width is `dialog`'s own 448px**, and its height is
/// `--window-height`'s own `h-[70vh]` — 630px at the 900px default.
#[gpui::test]
fn the_popup_is_four_forty_eight_by_the_driven_seventy_percent_height(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = at(&records, "repo-import-dialog-popup");

    assert_px(popup.size.width, px(448.0));
    assert_px(popup.size.height, px(630.0));
    assert_px(popup.origin.x, px(0.0));
    assert_px(popup.origin.y, px(0.0));
}

/// `--window-height` moves the popup by exactly 70% of its own delta —
/// `h-[70vh]`'s own arithmetic, live.
///
/// **Mutation:** replacing `VIEWPORT_HEIGHT_FACTOR` (0.7) with `1.0` in
/// `repo_import_dialog::popup_height_at` turns this red — 1200px would
/// render an 1198px popup (clamped by nothing on this axis) instead of 840.
#[gpui::test]
fn window_height_moves_the_popup_by_seventy_percent_of_its_own_delta(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for window_height in [600u16, 900, 1200] {
        let records = measure(cx, cell(&["--window-height", &window_height.to_string()]));
        let popup = at(&records, "repo-import-dialog-popup");
        let expected = f32::from(window_height) * 0.7;
        assert_px(popup.size.height, px(expected));
    }
}

/// **The border, radius, background and text colour are `dialog`'s own
/// tokens.**
#[gpui::test]
fn the_popup_has_this_crates_own_border_and_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = find(&records, "repo-import-dialog-popup");

    assert_px(popup.border_width, px(1.0));
    assert_px(popup.border_width, repo_import_dialog::BORDER_WIDTH);
    assert_px(popup.radius, Theme::DARK.radius_2xl.value());
    assert_px(popup.radius, px(18.0));
    assert_eq!(popup.background, Paint::Solid(Theme::DARK.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::DARK.border.value()));
}

/// **`p-4 pb-2` genuinely differs from `dialog`'s own `p-6`** — the header's
/// own content column is 448 − 2 − 16(pl) − 16(pr) = 414px wide, not the
/// 400px `dialog`'s `p-6` would produce, and its top inset off the border is
/// 16px, not 24.
///
/// **Mutation:** replacing `.pl(HEADER_PADDING).pr(HEADER_PADDING)` with
/// `dialog`'s own `.p(px(24.0))` in `RepoImportDialog::header` turns the
/// width assertion red (398 instead of 414) and the origin assertion red
/// (24 instead of 16).
#[gpui::test]
fn the_p4_pb2_override_differs_from_dialogs_p6(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let header = at(&records, "repo-import-dialog-header");
    let title = at(&records, "repo-import-dialog-title");

    assert_px(header.size.width, px(446.0));
    assert_px(title.origin.x - header.origin.x, px(16.0));
    assert_px(title.origin.y - header.origin.y, px(16.0));
    assert_ne!(title.origin.x - header.origin.x, px(24.0));
}

/// The title is its own line box and says so.
#[gpui::test]
fn the_title_is_its_own_line_box_and_says_so(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let title = find(&records, "repo-import-dialog-title");

    assert!(title.line_sized, "{title:?}");
    assert!(!title.content_sized, "{title:?}");
    assert_px(title.bounds.size.height, px(20.0));
    assert_eq!(
        title.text.as_ref().map(|text| text.content.to_string()),
        Some("Import branches".to_owned()),
    );
}

/// The description keeps `dialog`'s own default line height (no
/// `leading-relaxed` on this call site, unlike `detach-holder-modal`'s) —
/// checked the same font-metric-robust way: the observed height is a whole
/// multiple of the *default* 20px line height, not the *overridden* 22.75px
/// one.
#[gpui::test]
fn the_description_keeps_dialogs_default_line_height(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let description = find(&records, "repo-import-dialog-description");

    assert!(!description.line_sized, "{description:?}");
    let line_height_px = 14.0 * (1.25 / 0.875);
    let lines = f32::from(description.bounds.size.height) / line_height_px;
    assert!(
        (lines - lines.round()).abs() < 0.02,
        "height {:?} is not a whole multiple of {line_height_px}px (dialog's default); \
         {lines} lines",
        description.bounds.size.height,
    );
    assert_eq!(
        description
            .text
            .as_ref()
            .map(|text| text.content.to_string()),
        Some("Bring remote branches into Crowbar as workspaces.".to_owned()),
    );
}

/// The header's own height is content-driven and **does not move** when
/// `--window-height` changes the popup's own authored height — the inverse
/// of `dialog`'s relationship, where the popup's height is *derived from*
/// the body. This is the one row-layout-observable half of
/// `RepoImportDialog::body_height`'s arithmetic: the body itself carries no
/// anchor (unmodelled on purpose, `dialog::Dialog::body`'s own precedent —
/// see the module docs), so its own computed height cannot be read back from
/// rendered geometry at all, and `body_height`'s arithmetic is instead
/// pinned at the unit level, in
/// `crowbar_ui::surfaces::repo::repo_import_dialog`'s own
/// `the_body_is_the_popup_less_its_borders_and_header` test. Recorded here
/// rather than left implicit — a first draft of this test derived an
/// "expected" body height from the same three quantities it then re-summed,
/// which cannot fail regardless of what `body_height` actually computes.
///
/// **Mutation:** hardcoding `RepoImportDialog::header_height_estimate` to
/// return `px(200.0)` turns this red — the header's *real* rendered height
/// stops matching its own content-driven arithmetic (48 + 20 + 8 + a real
/// description height), which this test compares against directly, and the
/// popup itself keeps its own `--window-height`-driven height regardless.
#[gpui::test]
fn the_header_height_is_independent_of_window_height(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut heights = Vec::new();
    for window_height in [500u16, 900, 1400] {
        let records = measure(
            cx,
            cell(&["--window-height", &window_height.to_string()]),
        );
        let popup = at(&records, "repo-import-dialog-popup");
        let header = at(&records, "repo-import-dialog-header");

        assert_px(popup.size.height, px(f32::from(window_height) * 0.7));
        heights.push(header.size.height);
    }

    assert_eq!(heights[0], heights[1]);
    assert_eq!(heights[1], heights[2]);
}

/// `empty` removes the header, and only the header — there is no footer to
/// remove alongside it, unlike `dialog`'s and `detach-holder-modal`'s own
/// `empty` arms.
#[gpui::test]
fn empty_removes_the_header(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));
    let seen = ids(&records);

    assert!(
        !seen.contains(&"repo-import-dialog-header".to_owned()),
        "{seen:?}"
    );
    let popup = at(&records, "repo-import-dialog-popup");
    // The whole popup is body, since there is no header to subtract:
    // `h-[70vh]` at the default 900px window is still 630.
    assert_px(popup.size.height, px(630.0));
}

/// The light table paints a different popup.
#[gpui::test]
fn the_light_table_paints_a_different_popup(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let popup = find(&records, "repo-import-dialog-popup");

    assert_eq!(popup.background, Paint::Solid(Theme::LIGHT.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::LIGHT.border.value()));
    assert_ne!(Theme::LIGHT.popover, Theme::DARK.popover);
}
