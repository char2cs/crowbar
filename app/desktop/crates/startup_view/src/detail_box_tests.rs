use gpui::{TestAppContext, px};

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn a_long_report_stops_growing_at_the_maximum_height(cx: &mut TestAppContext) {
    let report = "line\n".repeat(80);
    let bounds = measure(cx, move |_, _| {
        div()
            .w(px(400.0))
            .child(DetailBox::new("box", report.clone()))
    });
    assert_eq!(f32::from(bounds.size.height), MAX_HEIGHT);
}

#[gpui::test]
fn a_short_message_hugs_its_text(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| {
        div()
            .w(px(400.0))
            .child(DetailBox::new("box", "step failed"))
    });
    assert!(f32::from(bounds.size.height) < MAX_HEIGHT, "{bounds:?}");
}
