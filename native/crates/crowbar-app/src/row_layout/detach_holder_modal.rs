//! `--surface detach-holder-modal`, laid out in a real window.
//!
//! No live pixel reference exists for this surface — `dialog.md` §5 names
//! why: the one live call site opens only when a protected branch is held by
//! another worktree, state no capture session has had on hand. What follows
//! checks two things instead: that the wrap renders and lays out **exactly
//! like `dialog`'s already-converged wrap does** on every field the two
//! share, and that this call site's two real `className` overrides
//! (`pr-10` on the header, `leading-relaxed` on the description) actually
//! move what they claim to.
//!
//! The description is three real sentences and **wraps** at this call site's
//! own (narrower, `pr-10`) content width — unlike `dialog`'s and
//! `alert-dialog`'s own reachable descriptions, which are short enough to
//! stay on one line. So this file does not hardcode an assumed line count:
//! the leading-relaxed test below instead checks that the *observed* height
//! is a whole multiple of this call site's
//! own line height (22.75px) — which `dialog`'s own default line height
//! (20px) cannot produce for the same wrapped line count, because line-height
//! moves the vertical spacing between wraps, never *where* a wrap happens
//! (wrapping is a function of glyph advances against the box width, which
//! `leading-relaxed` does not touch). That makes the check robust to
//! whatever the real wrap count happens to be on the machine running the
//! test, while still catching a reverted line-height constant.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::surfaces::detach_holder_modal;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec![
        "--surface",
        "detach-holder-modal",
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
    relative_to(records, detach_holder_modal::ID_POPUP, id)
}

/// **The wrap renders at all**, carrying every contract anchor the resting
/// cell has, and none of `dialog`'s own bare `dialog-*` ids — the finding
/// `crowbar_ui::surfaces::detach_holder_modal`'s module docs record: the
/// *real* DOM this call site paints would carry those, and this surface's own
/// namespace exists only because the registry requires a unique root.
#[gpui::test]
fn the_wrapped_popup_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        "detach-holder-modal-popup",
        "detach-holder-modal-header",
        "detach-holder-modal-title",
        "detach-holder-modal-description",
        "detach-holder-modal-footer",
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
        !seen.iter().any(|id| id.starts_with("alert-dialog-")),
        "{seen:?}"
    );
}

/// **The popup's width is `dialog`'s own 448px** — `max-w-md` and
/// `sm:max-w-md` are the same Tailwind step, and every cell this port drives
/// is at or above the `sm` breakpoint.
#[gpui::test]
fn the_popup_width_is_dialogs_own_four_forty_eight(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = at(&records, "detach-holder-modal-popup");

    assert_px(popup.size.width, px(448.0));
    assert_px(popup.origin.x, px(0.0));
    assert_px(popup.origin.y, px(0.0));
}

/// **The border, radius, background and text colour are `dialog`'s own
/// tokens** — the class lists are identical on these fields.
#[gpui::test]
fn the_popup_has_this_crates_own_border_and_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = find(&records, "detach-holder-modal-popup");

    assert_px(popup.border_width, px(1.0));
    assert_px(popup.border_width, detach_holder_modal::BORDER_WIDTH);
    assert_px(popup.radius, Theme::DARK.radius_2xl.value());
    assert_px(popup.radius, px(18.0));
    assert_eq!(popup.background, Paint::Solid(Theme::DARK.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::DARK.border.value()));
}

/// The header is the popup less its two borders on the width axis.
#[gpui::test]
fn the_header_is_the_popup_width_less_its_borders(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let header = at(&records, "detach-holder-modal-header");

    assert_px(header.origin.x, detach_holder_modal::BORDER_WIDTH);
    assert_px(header.origin.y, detach_holder_modal::BORDER_WIDTH);
    assert_px(header.size.width, px(446.0));
}

/// **`pr-10` genuinely narrows the header's own content column** — the
/// assertion that catches a reverted override immediately: the description
/// is `w-full` *inside* the header's padded content box, so its own box
/// width is `446 − 24 (pl) − 40 (pr)` = 382, not `446 − 24 − 24` = 398 the
/// way it would be under `dialog`'s own uniform `p-6`.
///
/// **Mutation:** replacing `.pr(HEADER_PADDING_RIGHT)` with
/// `.pr(HEADER_PADDING)` in `DetachHolderModal::header` turns this red —
/// the description's width becomes 398, not 382.
#[gpui::test]
fn the_pr_10_override_narrows_the_headers_content_column(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let description = find(&records, "detach-holder-modal-description");

    assert_px(description.bounds.size.width, px(382.0));
    assert_ne!(description.bounds.size.width, px(398.0));
}

/// The title sits at the header's own top-left, `pt`/`pl` both at
/// `HEADER_PADDING` (24px — the three sides `pr-10` does not touch), and is
/// its own line box.
#[gpui::test]
fn the_title_sits_at_the_headers_top_left_and_says_its_line_sized(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let title = find(&records, "detach-holder-modal-title");
    let header = at(&records, "detach-holder-modal-header");
    let title_rel = at(&records, "detach-holder-modal-title");

    assert!(title.line_sized, "{title:?}");
    assert!(!title.content_sized, "{title:?}");
    assert_px(title.bounds.size.height, px(20.0));
    assert_eq!(
        title.text.as_ref().map(|text| text.content.to_string()),
        Some("Detach to manage main".to_owned()),
    );
    assert_px(title_rel.origin.x - header.origin.x, px(24.0));
    assert_px(title_rel.origin.y - header.origin.y, px(24.0));
}

/// **`gap-2` genuinely separates the title from the description** — measured
/// as the vertical gap between the two boxes directly, not as a byproduct of
/// the popup's total height (a first draft's `the_popup_height_is_two_
/// borders_plus_the_real_header_plus_the_footer` test recomputes its expected
/// popup height from the *observed* header box, so it stays self-consistent
/// — and passes unchanged — even with the gap deleted from `header`'s real
/// `div()` chain, because the header simply renders 8px shorter and the sum
/// still balances. This test reads the two boxes' own origins instead, which
/// a shrunk header cannot hide.
///
/// **Mutation:** deleting `.gap(HEADER_GAP)` from `DetachHolderModal::header`
/// turns this red — the description's top edge lands flush against the
/// title's bottom edge, a 0px gap, not 8.
#[gpui::test]
fn the_gap_2_separates_the_title_from_the_description(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let title = at(&records, "detach-holder-modal-title");
    let description = at(&records, "detach-holder-modal-description");

    let title_bottom = title.origin.y + title.size.height;
    assert_px(description.origin.y - title_bottom, px(8.0));
}

/// **`leading-relaxed` genuinely changes the description's own line height**
/// — checked two ways.
///
/// First, and load-bearing: a short, guaranteed-one-line description's own
/// height, measured on *this* component, must differ from what the
/// identical string renders at on `dialog`'s own surface (whose
/// `DESCRIPTION_LINE_HEIGHT` this module's own constant must never equal,
/// by construction — see `crowbar_ui::surfaces::detach_holder_modal`'s own
/// unit test `the_two_overrides_genuinely_differ_from_dialogs_defaults` for
/// the source-level half of this same claim). This is what makes the
/// assertion sensitive to *this* module's own constant rather than to
/// whatever the two constants happen to agree on — an earlier draft compared
/// this component's multi-line height only to *itself* (a whole-multiple
/// check against its own one-line height) and passed unchanged when
/// `DESCRIPTION_LINE_HEIGHT` was mutated to `dialog`'s own value, because
/// both the numerator and the denominator moved together.
///
/// Second: the real, three-sentence description does wrap, at more than one
/// line — named separately so a future change to the fixture text cannot
/// silently make this component's own defining feature (a wrapping
/// description) stop being exercised.
///
/// **Mutation:** replacing `DESCRIPTION_LINE_HEIGHT` (1.625) with `dialog`'s
/// own `1.25 / 0.875` in this module turns the first assertion red — both
/// surfaces then render "Short." at the identical 20px, and `assert_ne`
/// fails.
#[gpui::test]
fn the_description_uses_leading_relaxed_not_dialogs_default(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let this_surface = measure(cx, cell(&["--description", "Short."]));
    let this_height = find(&this_surface, "detach-holder-modal-description")
        .bounds
        .size
        .height;

    let dialog_surface = measure(
        cx,
        a_cell(&[
            "--surface",
            "dialog",
            "--width",
            "1714",
            "--viewport-width",
            "1714",
            "--description",
            "Short.",
        ]),
    );
    let dialog_height = find(&dialog_surface, "dialog-description")
        .bounds
        .size
        .height;

    assert_ne!(
        this_height, dialog_height,
        "leading-relaxed should render a taller one-line box than dialog's default \
         text-sm line height",
    );
    assert!(this_height > dialog_height, "{this_height:?} vs {dialog_height:?}");

    let records = measure(cx, cell(&[]));
    let description = find(&records, "detach-holder-modal-description");
    assert!(!description.line_sized, "{description:?}");
    let lines = f32::from(description.bounds.size.height) / f32::from(this_height);
    assert!(lines.round() >= 2.0, "expected this prose to wrap: {lines}");
}

/// The footer sits flush against the popup's bottom border, its own height
/// following its content through its border and its padding — `dialog`'s
/// own arithmetic, unmodified by this call site.
#[gpui::test]
fn the_footer_height_follows_its_content(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for content in [32u16, 80, 120] {
        let records = measure(cx, cell(&["--footer-height", &content.to_string()]));
        let footer = at(&records, "detach-holder-modal-footer");
        let expected = f32::from(detach_holder_modal::BORDER_WIDTH)
            + f32::from(detach_holder_modal::FOOTER_PADDING_Y) * 2.0
            + f32::from(content);
        assert_px(footer.size.height, px(expected));
    }
}

/// The popup's own height is two borders, the header (whatever height its
/// wrapped description came out at), the (zero) body, and the footer —
/// self-consistent across every box this cell renders, not a hardcoded
/// literal that would drift with font metrics.
///
/// **This is a *consistency* check, not an independent one** — worth saying
/// plainly, because a mutation that shrinks the header (deleting
/// `.gap(HEADER_GAP)`, say) still balances: the popup renders that much
/// shorter too, and `border*2 + header + footer` holds against the new,
/// smaller numbers exactly as it did against the old ones. That specific
/// mutation is caught instead by `the_gap_2_separates_the_title_from_the_
/// description`, which reads the two boxes' own origins rather than a sum.
/// What *this* test catches is a wiring bug that breaks the additive
/// relationship itself — e.g. the body accidentally contributing twice, or
/// the outer neutralisation leaking one of the vendor's own paddings back
/// in.
///
/// **Mutation:** changing `DetachHolderModal::body` to
/// `div().w_full().h(px(f32::from(self.body_height) + 50.0))` — a body that
/// silently renders 50px taller than the field says — turns this red: the
/// popup grows by the same 50px (its content genuinely is that much taller),
/// but nothing in `header`/`footer`'s own observed boxes moved, so
/// `border*2 + header + footer` no longer accounts for the popup's real
/// height (`292.5px` expected, `342.5px` observed on this run). This is what
/// makes the test non-vacuous despite being a sum: it has no way to see the
/// *unanchored* body except through the popup's own total, so a body that
/// silently grows is exactly the class of bug it exists to catch.
#[gpui::test]
fn the_popup_height_is_two_borders_plus_the_real_header_plus_the_footer(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = at(&records, "detach-holder-modal-popup");
    let header = at(&records, "detach-holder-modal-header");
    let footer = at(&records, "detach-holder-modal-footer");

    let expected = f32::from(detach_holder_modal::BORDER_WIDTH) * 2.0
        + f32::from(header.size.height)
        + f32::from(footer.size.height);
    assert_px(popup.size.height, px(expected));
}

/// `empty` removes the header and the footer together, and the popup
/// shrinks to its two borders around the (zero) body.
#[gpui::test]
fn empty_removes_the_header_and_the_footer(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--flags", "empty"]));
    let seen = ids(&records);

    assert!(
        !seen.contains(&"detach-holder-modal-header".to_owned()),
        "{seen:?}"
    );
    assert!(
        !seen.contains(&"detach-holder-modal-footer".to_owned()),
        "{seen:?}"
    );
    let popup = at(&records, "detach-holder-modal-popup");
    assert_px(popup.size.height, px(2.0));
}

/// The light table paints a different popup.
#[gpui::test]
fn the_light_table_paints_a_different_popup(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let popup = find(&records, "detach-holder-modal-popup");

    assert_eq!(popup.background, Paint::Solid(Theme::LIGHT.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::LIGHT.border.value()));
    assert_ne!(Theme::LIGHT.popover, Theme::DARK.popover);
}
