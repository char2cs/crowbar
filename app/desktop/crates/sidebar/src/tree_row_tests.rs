use gpui::TestAppContext;

use super::*;
use crate::test_support::{chat, measure, recording_handler};

#[gpui::test]
fn a_row_is_the_spec_height_plus_its_vertical_margin(cx: &mut TestAppContext) {
    let (handler, _) = recording_handler();
    let bounds = measure(cx, move |_, _| TreeRow {
        row: chat("Refactor"),
        expansion: Expansion::Expanded,
        input: None,
        handler: Rc::clone(&handler),
    });
    assert_eq!(f32::from(bounds.size.height), 38.0);
}
