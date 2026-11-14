use gpui::TestAppContext;

use super::*;
use crate::test_support::measure;

#[gpui::test]
fn every_kind_renders_a_card(cx: &mut TestAppContext) {
    for kind in [ToastKind::Info, ToastKind::Success, ToastKind::Warning, ToastKind::Error, ToastKind::Loading] {
        let bounds = measure(cx, move |_, _| Toast::new("t", kind, "Branch created"));
        assert!(f32::from(bounds.size.height) >= 2.0 * 12.0 + 16.0, "{kind:?} keeps its padding");
    }
}
