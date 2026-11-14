use gpui::{Bounds, Pixels, TestAppContext, point, px, size};
use model::{DragSubject, Edge, Rect};

use super::*;
use crate::test_support::{chat, layout_of, pane_id};

const CONTENT: Rect = Rect {
    x: 0.0,
    y: 0.0,
    width: 806.0,
    height: 400.0,
};

fn bounds() -> Bounds<Pixels> {
    Bounds::new(point(px(0.0), px(0.0)), size(px(806.0), px(400.0)))
}

#[gpui::test]
fn dragging_over_a_pane_resolves_the_target_and_a_preview(cx: &mut TestAppContext) {
    let layout = layout_of("a", &[("b", "a", Edge::Right)]);
    let slots = layout.active_tab().expect("tab").layout(CONTENT);
    let subject = DragSubject::Open(chat("c"));
    let mut hover = DropHover::default();
    cx.update(|cx| {
        let hit = DropHit {
            subject: &subject,
            point: (410.0, 200.0),
            slots: &slots,
            window_bounds: bounds(),
        };
        hover.over(hit, cx);
    });
    assert!(hover.is_dragging(), "a drag is in progress");
    assert_eq!(
        hover.target.as_ref().map(|t| (t.pane.clone(), t.edge)),
        Some((pane_id("b"), Edge::Left)),
        "target"
    );
    assert!(
        hover.preview(Instant::now(), Motion::default()).is_some(),
        "a preview rectangle"
    );
}

#[gpui::test]
fn leaving_every_pane_drops_the_target_but_the_drag_goes_on(cx: &mut TestAppContext) {
    let layout = layout_of("a", &[("b", "a", Edge::Right)]);
    let slots = layout.active_tab().expect("tab").layout(CONTENT);
    let subject = DragSubject::Open(chat("c"));
    let mut hover = DropHover::default();
    cx.update(|cx| {
        let on = DropHit {
            subject: &subject,
            point: (410.0, 200.0),
            slots: &slots,
            window_bounds: bounds(),
        };
        hover.over(on, cx);
        let off = DropHit {
            point: (403.0, 200.0),
            ..on
        };
        hover.over(off, cx);
    });
    assert!(hover.is_dragging(), "still dragging");
    assert!(hover.target.is_none(), "no target over the sash");
    assert!(
        hover.preview(Instant::now(), Motion::default()).is_none(),
        "no preview"
    );
}

#[gpui::test]
fn clear_ends_the_drag(cx: &mut TestAppContext) {
    let layout = layout_of("a", &[]);
    let slots = layout.active_tab().expect("tab").layout(CONTENT);
    let subject = DragSubject::Open(chat("c"));
    let mut hover = DropHover::default();
    cx.update(|cx| {
        let hit = DropHit {
            subject: &subject,
            point: (10.0, 200.0),
            slots: &slots,
            window_bounds: bounds(),
        };
        hover.over(hit, cx);
    });
    hover.clear();
    assert!(!hover.is_dragging(), "no drag after clear");
}
