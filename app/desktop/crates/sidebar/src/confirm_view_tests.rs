use super::*;
use crate::test_support::prelude::*;

fn ask(sidebar: &gpui::Entity<Sidebar>, cx: &mut gpui::VisualTestContext) {
    sidebar.update(cx, |this, cx| {
        this.tree
            .request_project_removal(ProjectId::from("inv"), "inv");
        cx.notify();
    });
    cx.run_until_parked();
}

#[gpui::test]
fn cancelling_drops_the_question_and_sends_nothing(cx: &mut TestAppContext) {
    let (recorder, sidebar, cx) = open(cx, DemoData::rich());
    ask(&sidebar, cx);
    sidebar.update(cx, Sidebar::cancel_confirmation);
    assert!(sidebar.read_with(cx, |this, _| this.tree.confirmation().is_none()));
    assert_eq!(recorder.sent(), Vec::<Command>::new());
}

#[gpui::test]
fn accepting_deletes_the_space(cx: &mut TestAppContext) {
    let (recorder, sidebar, cx) = open(cx, DemoData::rich());
    ask(&sidebar, cx);
    sidebar.update(cx, Sidebar::accept_confirmation);
    assert_eq!(
        recorder.sent(),
        vec![Command::DeleteProject(ProjectId::from("inv"))]
    );
}

#[gpui::test]
fn the_layer_exists_only_while_a_question_is_pending(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::rich());
    assert!(!sidebar.read_with(cx, |this, _| this.tree.confirmation().is_some()));
    ask(&sidebar, cx);
    assert!(sidebar.read_with(cx, |this, _| this.tree.confirmation().is_some()));
}
