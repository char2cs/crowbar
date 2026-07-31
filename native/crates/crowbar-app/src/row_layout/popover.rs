//! `--surface popover`, laid out in a real window.
//!
//! What this file has to establish that no other one does: that a **wrapped**
//! `gpui-component` widget lays out where its React original does. Everything
//! measured here passes through `gpui_component::Popover`'s deferred, anchored
//! popup before it reaches the extractor, and the layers that wrap adds — a
//! `v_flex` with `.occlude()`, `.tab_group()` and a `top_1()` — are exactly the
//! kind of thing that could stretch or offset a box without anyone noticing.
//!
//! The measurements the assertions are written against were taken off the
//! **running React app** at a 1714px viewport, dark, with the popup pinned at
//! rest (`data-starting-style` removed and `transition: none`, the manoeuvre
//! `ANCHORS.md` v1.9 prescribes for an animated reference):
//!
//! ```text
//! popup     256 × 177    border 1px  radius 10  bg --popover  16px/24px
//! viewport  254 × 175    padding 16 on all four sides
//! ```

use super::{Stage, a_cell, assert_px, find, ids, relative_to};
use crowbar_driver::{AnchorRegistry, Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::popover;
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::driver_anchors::fold_text_halves;
use crate::row_surface::{Cell, RowSurface};

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "popover"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, popover::ID_POPUP, id)
}

/// **This surface needs two frames, and that is the item's finding.**
///
/// The shared `measure` is one `open_window` and a `run_until_parked`, which is
/// one frame — and every surface so far has been a pure function of its props,
/// so one frame was all any of them meant. `gpui_component::Popover` is not:
///
/// ```text
/// RenderOnce::render  →  if !open || !trigger_bounds_captured { return el; }
/// trigger's on_prepaint  →  trigger_bounds = bounds
///                           trigger_bounds_captured = true
///                           window.request_animation_frame()
/// ```
///
/// `trigger_bounds_captured` is set in the trigger's `on_prepaint`, which runs
/// *after* `render` has already returned — so frame 1 is the trigger and
/// nothing else, whatever `open` says. The vendor asks for another frame, and
/// on frame 2 the popup is in the tree.
///
/// `Window::request_animation_frame` is `on_next_frame(|_, cx| cx.notify(…))`,
/// and gpui's own comment on `simulate_next_frame` says why that is invisible
/// here: *"Tests have no platform frame loop"*. `run_until_parked` drives the
/// executor, not the frame loop, so the callback never fires and the shared
/// `measure` records **zero** anchors — measured, not inferred.
///
/// So this harness delivers the frame by hand. It is a local copy of
/// `measure_in` rather than a change to it, for two reasons: no other surface
/// should pay for a second frame it does not need, and the difference is
/// exactly the thing worth being able to see in one file.
///
/// **The driver binary has the same problem and no such escape** — see
/// `surfaces/popover.rs`'s module docs and `native/mapping/popover.md`.
fn measure(cx: &mut TestAppContext, cell: Cell) -> Vec<RawAnchor> {
    // `gpui_component::Popover` reaches `GlobalState` on the way to opening,
    // and that global is installed by `gpui_component::init` — which `main.rs`
    // calls and the shared harness does not.
    cx.update(gpui_component::init);

    let size = RowSurface::window_size(&cell);
    let anchors: AnchorRegistry = cx.update(crowbar_driver::install);
    let window = cx.open_window(size, |_, _| Stage(cell));
    cx.run_until_parked();

    window
        .update(cx, |_, window, cx| {
            let delivered = window.simulate_next_frame(cx);
            assert!(
                delivered > 0,
                "the vendor asked for no second frame, so either it stopped needing one \
                 or the popup stopped opening — both change what this file measures",
            );
        })
        .expect("the window is open");
    cx.run_until_parked();

    fold_text_halves(anchors.records())
}

/// **The wrap renders at all**, and it renders the popup rather than only the
/// trigger.
///
/// This is the first thing to check and the thing most likely to break on a
/// `gpui-component` bump: the popup is behind `if !open || !trigger_bounds_captured`,
/// and the second half of that is only true from the *second* frame.
/// `run_until_parked` drives the vendor's `request_animation_frame`, so it is
/// reached here — see the surface's module docs for where it is not.
#[gpui::test]
fn the_wrapped_popup_carries_every_contract_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in ["popover-popup", "popover-viewport"] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    // The reachable popover renders no title, so that anchor must be absent:
    // one the reference cannot produce is a `FieldPresence` delta.
    assert!(!seen.contains(&"popover-title".to_owned()), "{seen:?}");
    // And nothing from another surface leaked in — two roots in one frame would
    // make `Snapshot::build` anchor to whichever it found first.
    assert!(!seen.iter().any(|id| id.starts_with("menu-")), "{seen:?}");
    assert!(
        !seen.iter().any(|id| id.starts_with("git-row-")),
        "{seen:?}"
    );
}

/// **The popup box is the reference's, to the pixel**, and the vendor's own
/// wrapper neither stretches nor offsets it.
///
/// `render_popover_content` returns a `v_flex()`, whose default cross-axis
/// alignment is `stretch` — so a popup with no width of its own would have been
/// pulled to whatever that box resolved to. It has one, and this is the
/// assertion that says the declaration wins.
#[gpui::test]
fn the_popup_is_the_measured_two_fifty_six_by_one_seventy_seven(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = at(&records, "popover-popup");

    assert_px(popup.size.width, px(256.0));
    assert_px(popup.size.height, px(177.0));
    // The root sits at the origin of its own space, by construction (§4).
    assert_px(popup.origin.x, px(0.0));
    assert_px(popup.origin.y, px(0.0));
}

/// **The border is one real pixel**, which is the inverse of `dropdown-menu`'s
/// ring trap — and `border.w` is the field `ANCHORS.md` v1.1 compares
/// *exactly*, so getting it the other way round fails every cell.
#[gpui::test]
fn the_popup_has_a_real_one_pixel_border_and_a_ten_pixel_radius(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let popup = find(&records, "popover-popup");

    assert_px(popup.border_width, px(1.0));
    assert_px(popup.border_width, popover::BORDER_WIDTH);
    assert_px(popup.radius, Theme::DARK.radius_lg.value());
    assert_px(popup.radius, px(10.0));
    assert_eq!(popup.background, Paint::Solid(Theme::DARK.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::DARK.border.value()));
}

/// **The viewport inside the border**, which is the arithmetic a reader can
/// check: 256 − 1 − 1 = 254 wide, 177 − 1 − 1 = 175 tall, and it starts one
/// pixel in on both axes.
///
/// This is the assertion that would catch the vendor's wrapper taking room:
/// `.occlude()` and `.tab_group()` do not, and `top_1()` is on the layer
/// *above* the popup, so neither may move this box.
#[gpui::test]
fn the_viewport_is_the_popup_less_its_two_borders(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let viewport = at(&records, "popover-viewport");

    assert_px(viewport.origin.x, popover::BORDER_WIDTH);
    assert_px(viewport.origin.y, popover::BORDER_WIDTH);
    assert_px(viewport.size.width, px(254.0));
    assert_px(viewport.size.height, px(175.0));
}

/// The viewport's height **is** its padding plus the body, which is what makes
/// `--body-height` a legitimate stand-in for the call site's children rather
/// than a fudge factor.
#[gpui::test]
fn the_viewport_height_follows_the_body_through_its_padding(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for body in [40u16, 143, 300] {
        let records = measure(cx, cell(&["--body-height", &body.to_string()]));
        let viewport = at(&records, "popover-viewport");
        let expected = f32::from(popover::VIEWPORT_PADDING) * 2.0 + f32::from(body);
        assert_px(viewport.size.height, px(expected));

        // And the popup is that plus its two borders, on every one of them.
        let popup = at(&records, "popover-popup");
        assert_px(popup.size.height, px(expected + 2.0));
    }
}

/// `--popup-width` moves the popup and the viewport **together**, one border
/// apart — the property that says the viewport is `size-full` inside the popup
/// rather than a box with a width of its own.
#[gpui::test]
fn the_popup_width_reaches_the_viewport_through_the_border(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [200u16, 256, 420] {
        let records = measure(
            cx,
            cell(&["--popup-width", &width.to_string(), "--width", "480"]),
        );
        assert_px(
            at(&records, "popover-popup").size.width,
            px(f32::from(width)),
        );
        assert_px(
            at(&records, "popover-viewport").size.width,
            px(f32::from(width) - 2.0),
        );
    }
}

/// **The `tooltipStyle` arm is a different picture**, not a second spelling of
/// the same one: an 8px radius and a 4px viewport padding against 10 and 16.
///
/// It has no live call site and therefore no reference — the caption says so —
/// but the *port* has to be able to draw it, and a variant that quietly drew
/// the default would be worse than one that is declared unreachable.
#[gpui::test]
fn the_tooltip_variant_shrinks_the_radius_and_the_padding(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--tooltip"]));

    assert_px(find(&records, "popover-popup").radius, px(8.0));
    let viewport = at(&records, "popover-viewport");
    assert_px(viewport.origin.y, popover::BORDER_WIDTH);
    assert_px(
        viewport.size.height,
        px(f32::from(popover::TOOLTIP_VIEWPORT_PADDING_Y) * 2.0 + 143.0),
    );
}

/// The title, when a call site renders one: an 18px box at the top of the
/// viewport's padding, and `line_sized` declared on it.
///
/// The declaration is checked as well as the box, because `ANCHORS.md` v1.6
/// makes it a property the *component* asserts and the differ trusts — a
/// declaration that silently stopped travelling would open a blind spot and
/// announce nothing.
#[gpui::test]
fn the_title_is_its_own_line_box_and_says_so(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--title", "Merge into main"]));
    let title = find(&records, "popover-title");

    assert!(title.line_sized, "{title:?}");
    assert!(!title.content_sized, "{title:?}");
    assert_px(title.bounds.size.height, px(18.0));

    // It sits inside the viewport's padding, one border and 16px in.
    let relative = at(&records, "popover-title");
    assert_px(
        relative.origin.y,
        popover::BORDER_WIDTH + popover::VIEWPORT_PADDING,
    );
    assert_px(
        relative.origin.x,
        popover::BORDER_WIDTH + popover::VIEWPORT_PADDING,
    );

    // And the string travelled with it: what is recorded is what is painted.
    assert_eq!(
        title.text.as_ref().map(|text| text.content.to_string()),
        Some("Merge into main".to_owned()),
    );
}

/// The light table paints a different popup, so a theme cell on this surface is
/// not vacuous — `--popover` and `--border` both move.
#[gpui::test]
fn the_light_table_paints_a_different_popup(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--theme", "light"]));
    let popup = find(&records, "popover-popup");

    assert_eq!(popup.background, Paint::Solid(Theme::LIGHT.popover.value()));
    assert_eq!(popup.border_color, Some(Theme::LIGHT.border.value()));
    assert_ne!(Theme::LIGHT.popover, Theme::DARK.popover);
}
