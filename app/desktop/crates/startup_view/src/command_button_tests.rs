use std::rc::Rc;

use gpui::{Modifiers, TestAppContext, point, px};

use super::*;
use crate::test_support::{RecordingActions, open};

#[gpui::test]
fn a_click_performs_the_command(cx: &mut TestAppContext) {
    let actions = Rc::new(RecordingActions::default());
    let shared = Rc::clone(&actions) as Rc<dyn StartupActions>;
    let cx = open(cx, move |_, _| {
        CommandButton::new(StartupCommand::OpenLog, StartupCommand::Quit, &shared)
    });
    cx.simulate_click(point(px(20.0), px(16.0)), Modifiers::none());
    assert_eq!(actions.performed(), [StartupCommand::OpenLog]);
}
