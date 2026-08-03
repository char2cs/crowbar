//! `--surface search-replace-row`, laid out in a real window, against
//! `/tmp/p3-ref-search-replace-row.json` (P3.37 — see
//! `native/crates/crowbar-app/src/surfaces/search_replace_row.rs`'s module
//! docs for why this is a second surface rather than a flag on `search`).
//!
//! Built, not wrapped, like `search` itself, so this surface settles in the
//! **first** draw — `row_layout/search.rs`'s own comment establishes the
//! shape. What is worth a dedicated test here, on top of the geometry
//! `row_layout/search.rs`'s `the_replace_row_matches_the_captured_reference`
//! already pins for the *nested* rendering: that this surface's own registry
//! holds **only** this row's six anchors, never `search-popover`'s — the one
//! property `--surface search --replace` structurally cannot have (see the
//! module docs this file's sibling refers to) and the reason this surface
//! exists at all.

use super::{a_cell, assert_px, assert_within_tolerance, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::components::search;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied and
/// **`--width 306`** — the row's own measured width in
/// `/tmp/p3-ref-search-replace-row.json`, i.e. what it occupies nested
/// inside `search-popover`'s `320×`-wide, `p-1.5`-padded shell, its one
/// reachable context. `SearchReplaceRow`'s shell authors no width of its
/// own (no `w-[…]` — it is a plain flex row, sized entirely by its
/// container both here and there), so this surface's row is exactly as
/// wide as whatever container it is asked to fill; `Cell::default().width`
/// is `320` project-wide (the archived-gate rows' own figure, unrelated to
/// this surface), and rendering at that default instead of `306` moves the
/// flex-1 `Input` by a few tenths of a px against the reference for no
/// reason connected to this row's own layout.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "search-replace-row", "--width", "306"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to `search-replace-row`.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, search::ID_REPLACE_ROW, id)
}

/// **The whole point of this surface**: its registry is exactly the six
/// anchors `search-replace-row` and its children — `search-popover`,
/// `search-replace-toggle` and everything else `search`'s own tree carries
/// are simply never painted in this render pass, so they cannot leak in the
/// way `--surface search --replace`'s own registry cannot avoid (see the
/// module docs on the surface this file tests).
#[gpui::test]
fn the_registry_holds_only_this_rows_own_six_anchors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let mut seen = ids(&records);
    seen.sort();
    let mut expected = vec![
        "search-replace-row".to_owned(),
        "search-replace-icon".to_owned(),
        "input-control".to_owned(),
        "input".to_owned(),
        "search-replace-confirm".to_owned(),
        "search-replace-all".to_owned(),
    ];
    expected.sort();
    assert_eq!(seen, expected);

    // Named explicitly, not only implied by the count above: these are
    // exactly the anchors `--surface search --replace`'s own registry also
    // carries, and their *absence* here is the property under test.
    for absent in [
        "search-popover",
        "search-close",
        "search-toggle-case-sensitive",
    ] {
        assert!(!seen.contains(&absent.to_owned()), "{seen:?}");
    }
}

/// **The shell renders at all**, at its own origin (§4) — the frame boundary
/// is this row's own `AnchorSink::root` call
/// (`SearchReplaceRow::render_root`), not a nested `boxed` one. And at
/// `--width 306` its own box is `306×39`, matching
/// `/tmp/p3-ref-search-replace-row.json`'s root exactly.
#[gpui::test]
fn the_row_carries_its_root_anchor_at_its_own_origin(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let root = at(&records, "search-replace-row");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(306.0));
    assert_px(root.size.height, px(39.0));
}

/// **Matches `/tmp/p3-ref-search-replace-row.json`**: the swap icon at
/// `32×32`, and both labelled buttons content-sized with the strings the
/// reference has.
#[gpui::test]
fn the_default_cell_matches_the_captured_reference(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let icon = at(&records, "search-replace-icon");
    assert_px(icon.size.width, px(32.0));
    assert_px(icon.size.height, px(32.0));

    let confirm = find(&records, "search-replace-confirm");
    assert!(confirm.content_sized, "{confirm:?}");
    let confirm_text = confirm.text.expect("\"Replace\" paints a run");
    assert_eq!(confirm_text.content, "Replace");

    let all = find(&records, "search-replace-all");
    assert!(all.content_sized, "{all:?}");
    let all_text = all.text.expect("\"All\" paints a run");
    assert_eq!(all_text.content, "All");
}

/// **Defect 2 (P3.44).** The two labelled buttons take Tailwind's stock
/// `sm:text-sm` line-height (`calc(1.25rem / 0.875rem)` over `--ui-text-base`
/// = 14px = `20.0`px) — `button::Size::Default::type_step`'s own pair,
/// already used by every ordinary `Button::render` call — not gpui's
/// unset-style default, which reads `22.5` (measured live parity, not this
/// harness: no custom font is loaded here, so only the *ratio*, not the
/// shaped width, is safe to pin in this file — see
/// `this_rows_own_input_is_unclobbered`'s own comment for why).
///
/// Turns red on the one-line revert in `crates/crowbar-ui/src/components/
/// search.rs`'s `action_button`, deleting the `.line_height(relative(step.
/// line_height))` call (leaving `.text_size(step.size)` alone) — see the
/// P3.44 report's mutation evidence.
#[gpui::test]
fn the_labelled_buttons_take_text_sms_line_height(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let confirm = find(&records, "search-replace-confirm");
    let confirm_text = confirm.text.expect("\"Replace\" paints a run");
    assert_within_tolerance(confirm_text.font.line_height, px(20.0));

    let all = find(&records, "search-replace-all");
    let all_text = all.text.expect("\"All\" paints a run");
    assert_within_tolerance(all_text.font.line_height, px(20.0));
}

/// **Defect 1 (P3.44), the other half.** `SearchReplaceRow`'s own `Input`
/// override carries `py-1` but no `pl-*`/`pr-*` at all, so the field insets
/// from its control by only the control's constant 1px border horizontally
/// — not `search`'s own 33px — and by the same 5px vertically (border +
/// `py-1`'s 4px, shared by both call sites). `/tmp/p3-ref-search-replace-row
/// .json`'s own `input-control` `38,7,140.81,32` against `input`
/// `39,12,138.81,30`.
///
/// Turns red on the same one-line reverts
/// `row_layout::search::the_input_field_insets_from_its_control_by_the_icon_padding`
/// names — this test is what proves the two surfaces' insets actually
/// *differ*, which a shared hard-coded constant could not produce on either
/// side of the split.
#[gpui::test]
fn the_input_field_insets_from_its_control_by_only_the_border(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let control = at(&records, "input-control");
    let field = at(&records, "input");

    let left_inset = field.origin.x - control.origin.x;
    let right_inset = (control.origin.x + control.size.width) - (field.origin.x + field.size.width);
    let top_inset = field.origin.y - control.origin.y;

    assert_px(left_inset, px(1.0));
    assert_px(right_inset, px(1.0));
    assert_px(top_inset, px(5.0));
}

/// **This row's own `Input` is the narrower, undisturbed one, and not
/// `search`'s own `246×32` main field** — proof that this surface never
/// paints that other `Input` at all, so there is no second record able to
/// overwrite this one (the failure mode
/// `crowbar_ui::components::search::SearchReplaceRow::render_root`'s doc
/// comment describes for `--surface search --replace`).
///
/// The control's own **height** is asserted against the reference's exact
/// `32`/`30` — heights do not depend on the flex row's leftover space. Its
/// **width** is asserted by the same formula
/// `row_layout::search::the_replace_row_matches_the_captured_reference`
/// uses for the two action buttons, rather than against the reference's
/// literal `140.81`: this test harness has no custom font loaded (`main.rs`'s
/// `load_ui_font` only runs in the real binary), so `"Replace"`/`"All"` shape
/// a few px wider here than they did on the live, correctly-fonted capture —
/// confirmed by adding the identical debug print to both this test and
/// `search`'s own nested one and finding them equal (`81×32`/`48×32` in both,
/// not `75.81`/`39.38`). A hardcoded `140.81` would therefore fail for a
/// reason that has nothing to do with this surface's own correctness, which
/// is exactly the trap the formula sidesteps.
#[gpui::test]
fn this_rows_own_input_is_unclobbered(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let control = at(&records, "input-control");
    assert_px(control.size.height, px(32.0));
    let field = at(&records, "input");
    assert_px(field.size.height, px(30.0));
    assert_ne!(
        control.size.width,
        px(246.0),
        "search's own main field width leaked in"
    );

    let row = at(&records, "search-replace-row");
    let icon = at(&records, "search-replace-icon");
    let confirm = at(&records, "search-replace-confirm");
    let all = at(&records, "search-replace-all");
    let gaps = f32::from(search::SPACING_1_5) * 3.0; // icon–input, input–confirm, confirm–all
    let expected_control_width = f32::from(row.size.width)
        - f32::from(icon.size.width)
        - f32::from(confirm.size.width)
        - f32::from(all.size.width)
        - gaps;
    assert_px(control.size.width, px(expected_control_width));
}
