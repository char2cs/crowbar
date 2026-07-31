//! `--surface avatar`: what taffy resolves the avatar's circle to, in a real
//! window.
//!
//! Carried no module docs when it landed as an inline `mod` block; this header
//! is the file's, and nothing below it changed in the move. What the module is
//! about is a fixed `24 × 24` box, the radius `WebKit` prints for
//! `rounded-full`, and the fact that the two image states are two anchor sets.

use super::{a_cell, assert_px, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::avatar;
use crowbar_ui::components::avatar::{ALL_CALL_SITES, CallSite, FULL_RADIUS};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "avatar"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the avatar itself.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, avatar::ID_ROOT, id)
}

/// **The default cell is the live message avatar**, digit for digit against
/// the captured reference: `24 × 24`, `radius 3.4028234663852886e38`,
/// `border.w 0`, with the image mounted and the fallback gone.
#[gpui::test]
fn the_default_cell_is_the_live_message_avatar(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    assert_eq!(
        ids(&records),
        vec!["avatar".to_owned(), "avatar-image".to_owned()]
    );

    let root = at(&records, "avatar");
    assert_px(root.origin.x, px(0.0));
    assert_px(root.origin.y, px(0.0));
    assert_px(root.size.width, px(24.0));
    assert_px(root.size.height, px(24.0));

    let record = find(&records, "avatar");
    assert_eq!(record.radius, FULL_RADIUS);
    assert_px(record.border_width, px(0.0));
    assert!(record.visible);
    assert!(record.text.is_none(), "the root paints no text of its own");
    assert!(
        matches!(record.background, Paint::Solid(_)),
        "bg-background"
    );

    // `size-full` on the image: the same box, no paint, square corners.
    let image = at(&records, "avatar-image");
    assert_px(image.origin.x, px(0.0));
    assert_px(image.origin.y, px(0.0));
    assert_px(image.size.width, px(24.0));
    assert_px(image.size.height, px(24.0));
    let image_record = find(&records, "avatar-image");
    assert_px(image_record.radius, px(0.0));
    assert_px(image_record.border_width, px(0.0));
    assert_eq!(image_record.background, Paint::None);
}

/// `rounded-full` reaches the extractor as **`f32::MAX`**, which is what
/// `WebKit` computes for `calc(infinity * 1px)` — and is not gpui's own
/// `rounded_full()`.
///
/// Read off the recorded anchor rather than off the constant: a
/// `.rounded_full()` that slipped in would leave `FULL_RADIUS` right and put
/// 9999 in the snapshot, which is a 3.4e38 delta on a field compared at
/// ±0.5.
#[gpui::test]
fn the_recorded_radius_is_the_number_webkit_prints(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let recorded = f32::from(find(&records, "avatar").radius);
    assert!(
        (recorded - 340_282_346_638_528_859_811_704_183_484_516_925_440.0).abs() < f32::EPSILON,
        "the reference prints 340282346638528859811704183484516925440px, got {recorded}",
    );
    assert!(recorded > 9999.0, "gpui's rounded_full() would be 9999");
}

/// **The two image states are two anchor sets**, which is the finding this
/// surface exists to record. Neither carries both children, and `empty`
/// carries neither.
#[gpui::test]
fn the_image_status_decides_the_anchor_set(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let seen = ids(&measure(cx, cell(&["--image", "loaded"])));
    assert_eq!(seen, vec!["avatar".to_owned(), "avatar-image".to_owned()]);

    for status in ["pending", "absent"] {
        let seen = ids(&measure(cx, cell(&["--image", status])));
        assert_eq!(
            seen,
            vec!["avatar".to_owned(), "avatar-fallback".to_owned()],
            "{status}",
        );
    }

    // The `empty` cell: a root with neither child.
    let seen = ids(&measure(cx, cell(&["--flags", "empty"])));
    assert_eq!(seen, vec!["avatar".to_owned()]);
    // And reached from the other direction, which is the check that the two
    // spellings of "nothing in it" agree.
    let seen = ids(&measure(cx, cell(&["--image", "pending", "--no-fallback"])));
    assert_eq!(seen, vec!["avatar".to_owned()]);
}

/// The fallback fills the circle rather than sizing to its initials, which
/// is why it is in neither declaration list.
///
/// Measured against the captured probe: the agent message's fallback is
/// `24 × 24` around a 16px line box, and `bg-muted` is a real paint the root
/// does not have.
#[gpui::test]
fn the_fallback_is_size_full_and_neither_content_nor_line_sized(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&["--image", "pending"]));

    let fallback = at(&records, "avatar-fallback");
    assert_px(fallback.origin.x, px(0.0));
    assert_px(fallback.origin.y, px(0.0));
    assert_px(fallback.size.width, px(24.0));
    assert_px(fallback.size.height, px(24.0));

    let record = find(&records, "avatar-fallback");
    assert!(!record.content_sized);
    assert!(!record.line_sized);
    assert_eq!(record.radius, FULL_RADIUS);
    assert!(matches!(record.background, Paint::Solid(_)), "bg-muted");

    let text = record
        .text
        .clone()
        .expect("the fallback paints its initials");
    assert_eq!(text.content, "AG");
    // The probe reports `family: CalSansUI, size: 12, weight: 600` — the
    // call site's `font-semibold`, not the primitive's `font-medium`.
    assert_eq!(text.font.family, "CalSansUI");
    assert_px(text.font.size, px(12.0));
    assert!((text.font.weight - 600.0).abs() < f32::EPSILON);
    // 24 against a 16px line box: declaring `line_sized` would invent 8px.
    assert!((record.bounds.size.height - text.font.line_height).abs() > px(0.5));
    // And the run is narrower than the box, which is what makes the box an
    // authored square rather than a content-sized one.
    assert!(text.width < record.bounds.size.width);
}

/// A longer run **overflows** the circle rather than resizing it — the one
/// thing `--content` can show on this surface — and `overflow-hidden` on the
/// root is what clips it.
#[gpui::test]
fn a_long_fallback_overflows_the_authored_circle(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let mut boxes = Vec::new();
    for content in ["short", "normal", "overflow"] {
        let records = measure(cx, cell(&["--image", "pending", "--content", content]));
        let record = find(&records, "avatar-fallback");
        assert_px(record.bounds.size.width, px(24.0));
        boxes.push(record.text.clone().expect("initials").width);
    }

    assert!(boxes[0] < boxes[1] && boxes[1] < boxes[2], "{boxes:?}");
    assert!(boxes[2] > px(24.0), "the overflow arm has to overflow");
}

/// Every call-site bundle resolves to its own `size-*` and radius, and the
/// one with a finite corner is the one behind a popover.
#[gpui::test]
fn every_call_site_resolves_to_its_compiled_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let theme = Theme::DARK;

    for site in ALL_CALL_SITES {
        let records = measure(cx, cell(&["--call-site", site.name()]));
        let record = find(&records, "avatar");

        assert_px(record.bounds.size.width, site.extent());
        assert_px(record.bounds.size.height, site.extent());
        assert_eq!(record.radius, site.radius(&theme));
    }

    // `repo-icon` is the only live avatar whose radius is a number rather
    // than an infinity, and it is 14 — the token, not Tailwind's stock 12.
    let records = measure(cx, cell(&["--call-site", "repo-icon"]));
    assert_px(find(&records, "avatar").radius, px(14.0));
    assert_ne!(CallSite::RepoIcon.radius(&theme), FULL_RADIUS);
}

/// `--theme` is the only axis that moves anything here, and it moves two
/// paints: the root's `bg-background` and the fallback's `bg-muted`.
#[gpui::test]
fn the_theme_axis_moves_both_paints(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let dark = measure(cx, cell(&["--image", "pending", "--theme", "dark"]));
    let light = measure(cx, cell(&["--image", "pending", "--theme", "light"]));

    assert_ne!(
        find(&dark, "avatar").background,
        find(&light, "avatar").background,
    );
    assert_ne!(
        find(&dark, "avatar-fallback").background,
        find(&light, "avatar-fallback").background,
    );

    // And the width axis moves nothing, because every box is authored.
    let wide = measure(cx, cell(&["--width", "400"]));
    let narrow = measure(cx, cell(&["--width", "120"]));
    assert_eq!(
        at(&wide, "avatar").size,
        at(&narrow, "avatar").size,
        "size-6 is size-6 at every surface width",
    );
}
