use gpui::TestAppContext;

use crate::control::Control;
use crate::test_support::{inspect, open, row};

#[gpui::test]
fn the_row_button_is_enabled_when_the_backend_offers_its_capability(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "browser");
    let Control::Button(spec) = row("browserClearData").control else {
        panic!("browserClearData is a button row");
    };
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .button_enabled(&spec)));
}
