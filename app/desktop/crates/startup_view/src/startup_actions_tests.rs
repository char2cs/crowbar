use std::rc::Rc;

use gpui::TestAppContext;

use super::*;
use crate::test_support::RecordingActions;

#[gpui::test]
fn actions_are_usable_as_a_shared_trait_object(cx: &mut TestAppContext) {
    let recording = Rc::new(RecordingActions::default());
    let actions = Rc::clone(&recording) as Rc<dyn StartupActions>;
    cx.update(|cx| actions.perform(StartupCommand::OpenLog, cx));
    assert_eq!(recording.performed(), [StartupCommand::OpenLog]);
}
