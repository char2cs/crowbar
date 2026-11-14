use std::time::{Duration, Instant};

use gpui::TestAppContext;

use std::rc::Rc;

use super::*;
use crate::deadline::Deadline;
use crate::removal_kind::RemovalKind;
use crate::row_id::RowId;
use crate::test_support::{chat, measure, recording_handler};

#[gpui::test]
fn a_held_row_keeps_its_height_while_it_drains(cx: &mut TestAppContext) {
    let (handler, _) = recording_handler();
    let now = Instant::now();
    let hold = Hold {
        row: RowId::from("chat"),
        kind: RemovalKind::Chat,
        label: "x".into(),
        deadline: Deadline::At(now + Duration::from_secs(4)),
    };
    let bounds = measure(cx, move |_, _| RemovingRow {
        row: chat("x"),
        hold: hold.clone(),
        now,
        handler: Rc::clone(&handler),
    });
    assert_eq!(f32::from(bounds.size.height), 38.0);
}
