use std::rc::Rc;

use gpui::{SharedString, TestAppContext};
use ui::{Surface, Theme};

use super::*;
use crate::expansion::Expansion;
use crate::test_support::{chat, measure, recording_handler};

#[gpui::test]
fn the_cluster_is_laid_out_without_taking_flow_space(cx: &mut TestAppContext) {
    let (handler, _) = recording_handler();
    let row = chat("Refactor");
    let bounds = measure(cx, move |_, cx| {
        let spec = ClusterSpec {
            row: &row,
            group: SharedString::from("row"),
            expansion: Expansion::Expanded,
            palette: Theme::of(cx).palette(Surface::Chrome),
            handler: Rc::clone(&handler),
        };
        cluster(&spec)
    });
    assert_eq!(f32::from(bounds.size.height), 0.0);
}
