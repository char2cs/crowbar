//! `--surface nav-stack`, laid out in a real window.
//!
//! Pins the arithmetic `crowbar_ui::surfaces::sidebar::nav_stack`'s module docs
//! describe: the base layer's own recede, the pushed screen's header
//! reusing `sidebar_project_header`'s own height, and the claim that
//! neither the base layer nor a pushed screen's body depends on its own
//! (opaque) content.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::surfaces::sidebar::nav_stack;
use crowbar_ui::surfaces::sidebar::sidebar_project_header::{HEIGHT_MAC, HEIGHT_OTHER};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// The column height every cell below is measured at — `--height`'s own
/// default.
const HEIGHT: f32 = 600.0;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "nav-stack"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to this surface's own root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, nav_stack::ID_ROOT, id)
}

/// **The resting cell carries the root and the base layer, and nothing
/// else** — no screen is pushed, so none of the screen/header/back/title/
/// body anchors exist at all.
#[gpui::test]
fn the_resting_cell_carries_only_the_root_and_the_base_layer(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    assert_eq!(
        seen,
        vec![nav_stack::ID_ROOT.to_owned(), nav_stack::ID_BASE.to_owned()],
    );
}

/// **`--screen` pushes exactly one screen, carrying every one of its own
/// five anchors**, on top of the two the resting cell always carries.
#[gpui::test]
fn screen_pushes_the_full_header_and_body_anchor_set(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--screen"])));

    for id in [
        nav_stack::ID_ROOT,
        nav_stack::ID_BASE,
        nav_stack::ID_SCREEN,
        nav_stack::ID_HEADER,
        nav_stack::ID_BACK,
        nav_stack::ID_TITLE,
        nav_stack::ID_BODY,
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    assert_eq!(seen.len(), 7, "{seen:?}");
}

/// **The base layer recedes exactly a quarter of its own width when a
/// screen is pushed, and sits flush at rest.**
///
/// **Mutation:** flipping [`nav_stack::RECEDE_FRACTION`]'s sign (`0.25`
/// instead of `-0.25`) would move the base layer to the *right* instead of
/// the left — this test's own `assert!(receded.origin.x < 0.0)` catches
/// that directly, and the exact-quarter assertion catches a wrong
/// magnitude.
#[gpui::test]
fn the_base_layer_recedes_a_quarter_width_only_when_a_screen_is_pushed(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let resting = measure(cx, cell(&["--width", "294"]));
    let base_at_rest = at(&resting, nav_stack::ID_BASE);
    assert_px(base_at_rest.origin.x, px(0.0));

    let receded_records = measure(cx, cell(&["--width", "294", "--screen"]));
    let receded = at(&receded_records, nav_stack::ID_BASE);
    assert_px(receded.origin.x, px(-0.25 * 294.0));
    assert!(receded.origin.x < px(0.0));
}

/// **The pushed screen exactly fills the root** — `absolute inset-0`, so its
/// bounds are the root's own bounds regardless of the root's size.
#[gpui::test]
fn the_pushed_screen_exactly_fills_the_root(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for width in [240u16, 294, 420] {
        let records = measure(cx, cell(&["--width", &width.to_string(), "--screen"]));
        let root = find(&records, nav_stack::ID_ROOT);
        let screen = at(&records, nav_stack::ID_SCREEN);

        assert_px(screen.origin.x, px(0.0));
        assert_px(screen.origin.y, px(0.0));
        assert_px(screen.size.width, root.bounds.size.width);
        assert_px(screen.size.height, px(HEIGHT));
    }
}

/// **The header's own height follows the platform, and matches
/// `sidebar_project_header`'s exactly** — reused, not re-derived, per the
/// component's own module docs.
#[gpui::test]
fn the_header_height_matches_sidebar_project_header(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mac = measure(cx, cell(&["--screen"]));
    assert_px(at(&mac, nav_stack::ID_HEADER).size.height, HEIGHT_MAC);

    let other = measure(cx, cell(&["--screen", "--platform", "other"]));
    assert_px(at(&other, nav_stack::ID_HEADER).size.height, HEIGHT_OTHER);

    assert_ne!(HEIGHT_MAC, HEIGHT_OTHER);
}

/// **The traffic-light spacer needs both mac and left-docked** — a `--right`
/// cell's header starts closer to its own leading edge than a left-docked
/// one, because the 72px spacer **and the gap beside it** are both gone.
///
/// **The expected gap is 80px, not 72.** Removing the spacer does not just
/// remove its own 72px — it removes the `gap-2` (8px) that sat between it
/// and the back button too, since `gap` only applies *between* rendered
/// children. Left-docked: `HEADER_PADDING_X(12) + TRAFFIC_LIGHTS_WIDTH(72) +
/// HEADER_GAP(8) = 92`. Right-docked: `HEADER_PADDING_X(12) = 12`.
/// `92 − 12 = 80`. An earlier draft of this test asserted `72` and failed
/// with "expected 72px, got 80px" — the component's own gating was already
/// correct; the test's arithmetic had only counted the spacer's own width,
/// not the gap that disappears alongside it.
///
/// **Mutation:** dropping the `&& !isRight` half of the gate (rendering the
/// spacer whenever `IS_MAC`, regardless of dock side) would leave the back
/// button at the same `x` in both cells below — this test's own inequality
/// catches that; a mutation that only checked *presence* of the traffic
/// light anchor could not, since neither cell anchors it at all (it is
/// unpainted call-site geometry, matching `sidebar_project_header`'s own
/// spacer). Run and confirmed: reverting `!self.is_right` to `true` in
/// `NavStack::shows_traffic_lights` turns this red at
/// "expected 80px, got 0px" (both cells now render the spacer, so the two
/// `back` positions coincide).
#[gpui::test]
fn the_traffic_light_spacer_only_shows_up_mac_and_left_docked(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let left = measure(cx, cell(&["--screen"]));
    let right = measure(cx, cell(&["--screen", "--right"]));

    let back_left = at(&left, nav_stack::ID_BACK).origin.x;
    let back_right = at(&right, nav_stack::ID_BACK).origin.x;

    assert_px(back_left - back_right, px(80.0));
}

/// **The content filler moves nothing else** — the base layer's own box
/// does not depend on what is inside it, the same claim `sidebar-carousel`
/// makes about its panels.
#[gpui::test]
fn the_content_filler_does_not_move_the_base_layer(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let empty = measure(cx, cell(&["--width", "294"]));

    for filler in ["1", "1000"] {
        let stuffed = measure(cx, cell(&["--width", "294", "--content-width", filler]));
        assert_px(
            at(&stuffed, nav_stack::ID_BASE).size.width,
            at(&empty, nav_stack::ID_BASE).size.width,
        );
        assert_px(
            at(&stuffed, nav_stack::ID_BASE).origin.x,
            at(&empty, nav_stack::ID_BASE).origin.x,
        );
    }
}

/// **`--content` picks the pushed screen's own title text.**
#[gpui::test]
fn content_length_reaches_the_titles_own_text(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let short = measure(cx, cell(&["--screen", "--content", "short"]));
    let overflow = measure(cx, cell(&["--screen", "--content", "overflow"]));

    let short_text = find(&short, nav_stack::ID_TITLE)
        .text
        .expect("the title anchor paints text");
    assert_eq!(short_text.content.as_ref(), "Files");

    let overflow_text = find(&overflow, nav_stack::ID_TITLE)
        .text
        .expect("the title anchor paints text");
    assert_ne!(overflow_text.content, short_text.content);
}

/// **The height option reaches the root's own column.**
#[gpui::test]
fn the_height_option_sizes_the_column(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for height in [300u16, 900] {
        let records = measure(cx, cell(&["--height", &height.to_string()]));
        let root = find(&records, nav_stack::ID_ROOT);
        assert_px(root.bounds.size.height, px(f32::from(height)));
    }
}
