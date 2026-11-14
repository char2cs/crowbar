use std::time::{Duration, Instant};

use model::TabId;
use ui::Motion;

use super::*;

#[test]
fn the_drop_line_glides_to_its_new_position_in_sixty_milliseconds() {
    let start = Instant::now();
    let mut incoming = IncomingDrop::new(TabDrop::Reorder(0), 0.0);
    incoming.move_to(TabDrop::Merge(TabId(1)), 100.0, (start, Motion::default()));
    assert_eq!(incoming.drop, TabDrop::Merge(TabId(1)), "new target");
    assert_eq!(
        incoming.line_x(start, Motion::default()),
        0.0,
        "starts where it was"
    );
    assert!(incoming.is_moving(start, Motion::default()), "moving");
    let end = start + Duration::from_millis(60);
    assert_eq!(incoming.line_x(end, Motion::default()), 100.0, "arrived");
    assert!(!incoming.is_moving(end, Motion::default()), "stopped");
}
