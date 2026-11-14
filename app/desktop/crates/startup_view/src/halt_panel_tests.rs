use std::rc::Rc;

use gpui::{TestAppContext, px};

use super::*;
use crate::test_support::{RecordingActions, measure};

fn panel(code: Option<&str>) -> HaltPanel {
    let actions: Rc<dyn StartupActions> = Rc::new(RecordingActions::default());
    HaltPanel::new(&HaltReason::from_code(code), "step 3 failed", &actions)
}

#[gpui::test]
fn the_card_has_a_fixed_width(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| panel(Some("step_failed")));
    assert_eq!(f32::from(bounds.size.width), WIDTH);
}

#[gpui::test]
fn a_known_code_adds_a_reason_badge(cx: &mut TestAppContext) {
    let coded = measure(cx, |_, _| panel(Some("layout_newer")));
    let uncoded = measure(cx, |_, _| panel(None));
    assert!(coded.size.height > uncoded.size.height + px(10.0));
}
