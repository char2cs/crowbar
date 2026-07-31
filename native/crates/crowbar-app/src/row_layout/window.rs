//! **The driver's window follows the surface** — P2.5, measured.
//!
//! Every other module here measures a component. This one measures the *driver*:
//! whether the window it opens is big enough for the picture it was asked to
//! draw, and what happens when it is not.
//!
//! The item exists because it was not. `crowbar-app` capped `--shell-height` at
//! `1..=160` on `resizable` and `--height` at `1..=640` on `sidebar-carousel`,
//! on grounds that were correct — a surface cut by the driver's own window makes
//! every `visible` in the snapshot an artefact of the window size, which is
//! worse than refusing — but applied to the wrong side of the equation. The live
//! IDE shell's `ResizablePanelGroup` is **1119px** tall, so the cap made the
//! only real reference unreachable and `resizable` could not be parity-tested at
//! all.
//!
//! So the window moves. What the caps protected is kept twice over, and both
//! halves are measured below: the surface is **cut and never squashed** when a
//! window comes back short, and a frame with a cut anchor in it is **refused
//! rather than emitted**.

use super::{a_cell, assert_px, find, ids, measure};
use crowbar_driver::{AnchorRegistry, RawAnchor};
use crowbar_ui::components::resizable;
use crowbar_ui::components::sidebar_carousel::{ID_SCROLLPORT, SidebarTab, TABS};
use gpui::{Pixels, Size, TestAppContext, px, size};

use crate::driver_anchors::{DriverAnchors, fold_text_halves};
use crate::row_snapshot::{Destination, emit};
use crate::row_surface::{Cell, INSET_Y, RowSurface};

/// The live IDE shell's own measured height, in logical px.
///
/// The number this whole item is about. It came off the running app —
/// `web/src/components/layout/ide-shell.tsx`'s `ResizablePanelGroup` in its
/// `h-screen` box — which is why it is a constant here rather than a round
/// number chosen to be large.
const LIVE_SHELL_HEIGHT: u16 = 1119;

/// Lays a cell out through **`RowSurface` itself** — the view the binary
/// opens, caption and `size_full` root and all — in a window of the given
/// size, and hands back the registry as well as the records.
///
/// Not `super::measure`, and the difference is the point of this module.
/// `Stage` is an auto-sized root, so nothing about a window too short for
/// its content is visible through it. `RowSurface`'s root is `size_full`,
/// which makes it a flex container with the window's *definite* height — and
/// that is where a surface can be quietly compressed instead of cut.
fn draw(
    cx: &mut TestAppContext,
    cell: &Cell,
    window: Size<Pixels>,
) -> (AnchorRegistry, Vec<RawAnchor>) {
    let anchors: AnchorRegistry = cx.update(crowbar_driver::install);
    let caption = cell.describe();
    let subject = cell.clone();
    let _window = cx.open_window(window, |_, _| {
        RowSurface::new(subject, Box::new(DriverAnchors), caption)
    });
    cx.run_until_parked();
    let records = fold_text_halves(anchors.records());
    (anchors, records)
}

/// The window the app would open for this cell.
fn window_for(cell: &Cell) -> Size<Pixels> {
    RowSurface::window_size(cell)
}

/// **The live IDE shell's height, drawn unclipped.**
///
/// The headline assertion of P2.5: at `--shell-height 1119` — the height the
/// old cap of 160 refused — every anchor on the surface reports
/// `visible: true` and carries the geometry the CSS arithmetic asks for. Not
/// "the parser permitted it": permitted-but-clipped is precisely the outcome
/// the cap existed to prevent, and it would show up here as a `visible` that
/// went false or a height that came back short.
#[gpui::test]
fn the_live_shell_height_is_drawn_with_every_anchor_visible(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = a_cell(&[
        "--surface",
        "resizable",
        "--width",
        "600",
        "--viewport-width",
        "700",
        "--shell-height",
        "1119",
    ]);

    // The window grew to hold it, rather than the surface being held down to
    // the window: 16px inset, the group, the caption, 16px inset.
    let window = window_for(&cell);
    assert_px(window.height, px(INSET_Y * 2.0 + 1119.0 + 29.0));
    assert!(window.height > px(f32::from(LIVE_SHELL_HEIGHT)));

    let (_anchors, records) = draw(cx, &cell, window);
    let group = find(&records, resizable::ID_GROUP);

    // Every anchor visible — the field the cap was protecting.
    for id in ids(&records) {
        assert!(find(&records, &id).visible, "{id} is not visible");
    }
    assert_eq!(ids(&records).len(), 4, "{:?}", ids(&records));

    // …and genuinely 1119 tall, top to bottom, inside the window.
    assert_px(group.bounds.size.height, px(1119.0));
    assert_px(group.bounds.origin.y, px(INSET_Y));
    assert_px(group.bounds.origin.y + group.bounds.size.height, px(1135.0));
    assert!(px(1135.0) <= window.height);

    // The two panels and the separator stretch the group's full height, so
    // the height reaches every anchor and not only the root.
    for id in [
        "resize-panel-sidebar",
        "resize-handle",
        "resize-panel-content",
    ] {
        let anchor = find(&records, id);
        assert_px(anchor.bounds.size.height, px(1119.0));
        assert_px(anchor.bounds.origin.y, px(INSET_Y));
        assert!(anchor.visible, "{id}");
    }
}

/// **The other surface, at the same height.** `sidebar-carousel` was capped
/// at 640 for the same reason and is freed by the same change — including
/// the one anchor that is legitimately invisible here, so this is not "every
/// anchor is visible" passing because the driver forgot how to say false.
#[gpui::test]
fn the_carousel_is_drawn_at_the_same_height_unclipped(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = a_cell(&[
        "--surface",
        "sidebar-carousel",
        "--width",
        "294",
        "--viewport-width",
        "600",
        "--height",
        "1119",
    ]);

    let window = window_for(&cell);
    assert_px(window.height, px(INSET_Y * 2.0 + 1119.0 + 29.0));

    let (_anchors, records) = draw(cx, &cell, window);

    // The scrollport and all four panels are the full 1119 tall, none of
    // them cut, and — at the resting track — every one of them visible.
    for id in [ID_SCROLLPORT]
        .into_iter()
        .chain(TABS.into_iter().map(SidebarTab::anchor))
    {
        let anchor = find(&records, id);
        assert_px(anchor.bounds.size.height, px(1119.0));
        assert_px(anchor.bounds.origin.y, px(INSET_Y));
    }
    assert!(find(&records, ID_SCROLLPORT).visible);
    assert!(find(&records, SidebarTab::Workspaces.anchor()).visible);

    // …and the three scrolled out of the scrollport horizontally are still
    // reported invisible, which is what proves the assertion above is
    // reading a live field rather than a constant.
    for tab in [SidebarTab::Chats, SidebarTab::Files, SidebarTab::Git] {
        assert!(!find(&records, tab.anchor()).visible, "{}", tab.name());
    }
}

/// **A cut frame is refused, not written.**
///
/// The property the old caps were protecting, kept where it can actually be
/// observed. `emit` takes the drawable area the platform *granted* and
/// checks the frame it drew against it, so a snapshot whose `visible` fields
/// would be artefacts of the window size is never produced — and the
/// refusal names the anchor, both numbers and what it would have cost.
///
/// The first two assertions are the **precondition**, and they are stated
/// rather than assumed: the refusal works by finding an anchor outside the
/// window, so it has teeth only if a short window *cuts* the surface instead
/// of compressing it into itself. A squashed surface would report a full set
/// of plausible bounds that are the wrong picture and leave nothing outside
/// the window at all. They are here rather than in a test of their own
/// because no mutation of this crate can make them fail — see the note in
/// `RowSurface::render` — and a test nothing can break is not a test.
#[gpui::test]
fn a_cut_surface_is_refused_and_nothing_is_written(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = a_cell(&[
        "--surface",
        "resizable",
        "--width",
        "600",
        "--viewport-width",
        "700",
        "--shell-height",
        "1119",
    ]);

    // A window a third of what the cell asked for — which is what macOS
    // hands back on a display that cannot hold it.
    let short = size(window_for(&cell).width, px(400.0));
    let (anchors, records) = draw(cx, &cell, short);

    let group = find(&records, resizable::ID_GROUP);
    assert_px(group.bounds.size.height, px(1119.0));
    assert!(
        group.bounds.origin.y + group.bounds.size.height > short.height,
        "the group must reach past the window, not be squeezed inside it",
    );

    // A path that does not exist, so "nothing was written" is checkable
    // without a temporary file to clean up.
    let refused = std::env::temp_dir().join("crowbar-p25-refused-must-not-exist.json");
    let _ = std::fs::remove_file(&refused);

    let outcome = emit(&anchors, &cell, &Destination::File(refused.clone()), short);

    let Err(complaint) = outcome else {
        panic!("a surface cut by the window must not be emitted");
    };
    assert!(complaint.contains("resize-group"), "{complaint}");
    assert!(complaint.contains("artefact"), "{complaint}");
    assert!(complaint.contains("400"), "{complaint}");
    assert!(complaint.contains("1135"), "{complaint}");
    // And the number to act on: the tallest surface this window *would*
    // have held, which is the window less the inset the surface starts at.
    // Without it a reader has to bisect `--shell-height` by hand.
    assert!(complaint.contains("384"), "{complaint}");
    assert!(!refused.exists(), "a refused emit wrote {refused:?}");
}

/// **The control the refusal above needs.** Without it, "the emit failed"
/// would also pass on an `emit` that had stopped working altogether — which
/// is the shape of vacuous guard that has already cost this project a
/// batch of tests. The identical cell in the window it asked for emits, and
/// the file is there.
#[gpui::test]
fn the_same_cell_in_the_window_it_asked_for_emits(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = a_cell(&[
        "--surface",
        "resizable",
        "--width",
        "600",
        "--viewport-width",
        "700",
        "--shell-height",
        "1119",
    ]);

    let window = window_for(&cell);
    let (anchors, _records) = draw(cx, &cell, window);

    let written = std::env::temp_dir().join("crowbar-p25-emitted.json");
    let _ = std::fs::remove_file(&written);

    let path = emit(&anchors, &cell, &Destination::File(written.clone()), window)
        .expect("a surface the window holds is emitted");
    assert_eq!(path, written);

    let json = std::fs::read_to_string(&written).expect("the snapshot is on disk");
    assert!(json.contains("\"resize-group\""), "{json}");
    // And every anchor in it says `visible`, which is the whole reason the
    // window had to grow.
    assert!(!json.contains("\"visible\": false"), "{json}");
    let _ = std::fs::remove_file(&written);
}

/// The concrete P2.12 cell: the live IDE shell, edge to edge.
///
/// `innerWidth` 1200 with a 1200×800 `resize-group`, and the growth factors
/// `react-resizable-panels` resolved. Every number below came off the running
/// web app, which is why they are literals here rather than arithmetic.
fn full_bleed_cell() -> Cell {
    a_cell(&[
        "--surface",
        "resizable",
        "--viewport-width",
        "1200",
        "--width",
        "1200",
        "--theme",
        "dark",
        "--content",
        "normal",
        "--shell-height",
        "800",
        "--grow",
        "24.521,75.478996",
    ])
}

/// The three panel ids the live group carries, and their measured widths.
const LIVE_DIVISION: [(&str, f32); 3] = [
    ("resize-panel-sidebar", 294.0),
    ("resize-handle", 1.0),
    ("resize-panel-content", 905.0),
];

/// **A surface that fills its viewport is drawn whole** — P2.12, measured.
///
/// The width counterpart of `the_live_shell_height_is_drawn_with_every_anchor_visible`.
/// The reference here is the IDE shell **root**: it fills its window, so the
/// surface width *is* the viewport width, and the driver's 24px horizontal
/// inset — free for every row narrower than its viewport — would push it
/// past the window edge. A full-bleed surface takes none of it.
///
/// **`visible` alone would not carry this claim, so containment does.** The
/// driver's `is_visible` is an intersection test, so a box hanging 24px past
/// the right edge of its window still reports `visible: true` — which is
/// exactly what the old geometry would have produced here. So every anchor
/// is asserted to lie *inside* the window rather than merely to overlap it,
/// and `the_same_cell_in_a_window_narrower_than_the_surface_is_cut` is the
/// control that proves both halves of that are reading live quantities.
#[gpui::test]
fn a_full_bleed_surface_at_its_viewport_width_is_drawn_unclipped(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = full_bleed_cell();
    assert_eq!(cell.horizontal_inset(), 0);

    // The window is the viewport verbatim, and the surface is its whole
    // width: one number, which is the thing that was unrepresentable.
    let window = window_for(&cell);
    assert_px(window.width, px(1200.0));
    assert_px(window.width, cell.width_px());

    let (_anchors, records) = draw(cx, &cell, window);
    assert_eq!(ids(&records).len(), 4, "{:?}", ids(&records));

    for id in ids(&records) {
        let anchor = find(&records, &id);
        assert!(anchor.visible, "{id} is not visible");
        // Inside the window, not merely intersecting it.
        assert!(
            anchor.bounds.origin.x >= px(0.0),
            "{id} starts left of the window at {:?}",
            anchor.bounds.origin.x,
        );
        assert!(
            anchor.bounds.origin.x + anchor.bounds.size.width <= window.width,
            "{id} reaches {:?} across a {:?} window",
            anchor.bounds.origin.x + anchor.bounds.size.width,
            window.width,
        );
    }

    // The root sits flush with the window's left edge, which is where the
    // reference's own root sits, and is 1200×800.
    let group = find(&records, resizable::ID_GROUP);
    assert_px(group.bounds.origin.x, px(0.0));
    assert_px(group.bounds.origin.y, px(INSET_Y));
    assert_px(group.bounds.size.width, px(1200.0));
    assert_px(group.bounds.size.height, px(800.0));

    // …and the division inside it is the live one, to the pixel.
    for (id, width) in LIVE_DIVISION {
        let anchor = find(&records, id);
        assert_px(anchor.bounds.size.width, px(width));
        assert_px(anchor.bounds.size.height, px(800.0));
        assert_px(anchor.bounds.origin.y, px(INSET_Y));
    }
    assert_px(
        find(&records, "resize-panel-sidebar").bounds.origin.x,
        px(0.0),
    );
    assert_px(find(&records, "resize-handle").bounds.origin.x, px(294.0));
    assert_px(
        find(&records, "resize-panel-content").bounds.origin.x,
        px(295.0),
    );
}

/// **The control the assertion above needs.** The identical cell in a window
/// narrower than the surface *is* cut, and every anchor still says
/// `visible: true` while it happens — which is why the test above asserts
/// containment and not visibility, and is the shape of evidence a
/// "permitted, therefore fine" claim cannot produce.
#[gpui::test]
fn the_same_cell_in_a_window_narrower_than_the_surface_is_cut(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = full_bleed_cell();
    // 200px short of the surface — which is what the driver would have been
    // drawing had the inset stayed, only more of it.
    let narrow = size(px(1000.0), window_for(&cell).height);

    let (_anchors, records) = draw(cx, &cell, narrow);
    let group = find(&records, resizable::ID_GROUP);

    // Cut, not squashed: the surface keeps its 1200px and reaches past the
    // edge, exactly as it keeps its height in the vertical case.
    assert_px(group.bounds.size.width, px(1200.0));
    assert!(
        group.bounds.origin.x + group.bounds.size.width > narrow.width,
        "the group must reach past the window to have anything to prove",
    );

    // And `visible` is *true* throughout, on a frame that is plainly cut.
    for id in ids(&records) {
        assert!(
            find(&records, &id).visible,
            "{id}: `visible` is an intersection test, so a cut box still reports true",
        );
    }
}

/// **What the caption does to the width axis: nothing.**
///
/// It is the reason the window has to exceed the surface *vertically*
/// (`CAPTION_HEIGHT`), so the question is fair — and the answer is measured
/// rather than argued. The caption is a block-level sibling *below* the
/// surface inside a root gpui lays out as `display: block`, so it is never a
/// flex item competing for the main axis, and it carries no anchor, so it
/// cannot reach a snapshot either. The proof is that the same cell measured
/// through `RowSurface` — caption and `size_full` root and all — and through
/// the caption-less `Stage` records the identical frame, anchor for anchor,
/// at the width where a horizontal interaction would be loudest: the one
/// where the surface has the whole window.
///
/// The one thing it does share is the root's left padding, so a full-bleed
/// cell draws its caption flush at x = 0 too. That is chrome moving with the
/// surface it describes, and it is why the inset is applied once on the root
/// rather than twice.
#[gpui::test]
fn the_caption_has_no_horizontal_effect_on_the_surface(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let cell = full_bleed_cell();

    let (_anchors, with_caption) = draw(cx, &cell, window_for(&cell));
    let without_caption = measure(cx, cell);

    assert_eq!(with_caption, without_caption);
    // Stated as well as compared, so a future frame that recorded nothing at
    // all could not satisfy the equality.
    assert_eq!(ids(&with_caption).len(), 4, "{:?}", ids(&with_caption));
}

/// **The two Phase 1 surfaces' window is the one their archived runs were
/// taken at**, to the pixel — because neither drives a height, so the floor
/// is the whole arithmetic and `window_extent` cannot move it.
///
/// 24 snapshot pairs in `native/oracle/runs/` were measured at this
/// geometry. A change here is a re-baselining, and re-baselining archived
/// evidence is not a thing this port does.
#[gpui::test]
fn the_phase_one_surfaces_keep_the_window_they_were_measured_in(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for surface in ["git-status-row", "file-tree-row"] {
        let cell = a_cell(&["--surface", surface]);
        assert_eq!(cell.window_extent(), 72, "{surface}");
        assert_px(window_for(&cell).height, px(104.0));
        // And their **horizontal** geometry likewise, which is what P2.12
        // had to leave alone: neither is full-bleed, so both keep the 24px
        // inset their archived runs were taken at. A surface that acquired
        // `full_bleed` by accident would move the root anchor to x = 0.
        assert_eq!(cell.horizontal_inset(), 24, "{surface}");
        assert_px(
            find(&draw(cx, &cell, window_for(&cell)).1, cell.surface.root)
                .bounds
                .origin
                .x,
            px(24.0),
        );

        // And what they emit is untouched by the window having become a
        // computed quantity: the same records, anchor for anchor, as in the
        // 400px window the harness used before P2.5.
        let (_, now) = draw(cx, &cell, window_for(&cell));
        let (_, before) = draw(cx, &cell, size(cell.viewport_width_px(), px(400.0)));
        assert_eq!(now, before, "{surface}");
    }
}
