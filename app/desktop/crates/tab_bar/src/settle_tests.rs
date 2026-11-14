use std::time::{Duration, Instant};

use model::TabId;
use ui::Motion;

use super::*;

#[test]
fn the_dropped_tab_starts_at_the_ghost_offset_and_ends_in_its_slot() {
    let start = Instant::now();
    let settle = Settle::new(TabId(3), 80.0, start, Motion::default());
    assert_eq!(
        settle.offset(start, Motion::default()),
        80.0,
        "starts at the ghost"
    );
    assert!(settle.is_running(start, Motion::default()), "gliding");
    let end = start + Duration::from_millis(150);
    assert_eq!(settle.offset(end, Motion::default()), 0.0, "in its slot");
    assert!(!settle.is_running(end, Motion::default()), "done");
}
