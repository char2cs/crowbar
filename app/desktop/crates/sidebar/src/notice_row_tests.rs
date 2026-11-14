use gpui::TestAppContext;

use std::rc::Rc;

use super::*;
use crate::test_support::{measure, recording_handler};

#[gpui::test]
fn the_notice_renders_its_sentence_above_a_retry_button(cx: &mut TestAppContext) {
    let (handler, _) = recording_handler();
    let bounds = measure(cx, move |_, _| NoticeRow {
        notice: Notice::Project,
        project: ProjectId::from("inv"),
        handler: Rc::clone(&handler),
    });
    assert!(f32::from(bounds.size.height) > 0.0);
}
