use std::time::{Duration, Instant};

use gpui::{point, px};
use model::TabId;
use ui::Motion;

use super::*;
use crate::span::Span;

fn session() -> DragSession {
    let tabs: Vec<TabId> = (0..3).map(TabId).collect();
    let spans: Vec<Span> = (0..3u16)
        .map(|i| Span {
            x: f32::from(i) * 100.0,
            width: 100.0,
        })
        .collect();
    let engine = StripDrag::begin(tabs, spans, 0, 50.0).expect("engine");
    DragSession::new(engine, point(px(50.0), px(20.0)), 16.0, 3)
}

#[test]
fn neighbours_slide_over_150_ms_when_the_target_moves() {
    let start = Instant::now();
    let mut session = session();
    session.engine.moved(160.0);
    session.follow(point(px(160.0), px(20.0)), start, Motion::default());
    assert_eq!(
        session.slide(1, start, Motion::default()),
        0.0,
        "starts in place"
    );
    assert!(session.is_sliding(start, Motion::default()), "sliding");
    let settled = start + Duration::from_millis(150);
    assert_eq!(
        session.slide(1, settled, Motion::default()),
        -100.0,
        "one pitch to the left"
    );
    assert_eq!(
        session.slide(0, settled, Motion::default()),
        0.0,
        "the dragged tab stays at zero"
    );
    assert!(!session.is_sliding(settled, Motion::default()), "done");
}
