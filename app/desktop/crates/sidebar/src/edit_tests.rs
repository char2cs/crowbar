use super::*;
use crate::test_support::prelude::*;

fn type_name(sidebar: &gpui::Entity<Sidebar>, name: &str, cx: &mut gpui::VisualTestContext) {
    let field = sidebar.read_with(cx, |this, _| {
        this.inline.as_ref().map(|edit| edit.field.clone())
    });
    let field = field.expect("an input is open");
    field.update(cx, |field, cx| field.set_text(name, cx));
}

fn start(sidebar: &gpui::Entity<Sidebar>, cx: &mut gpui::VisualTestContext) {
    sidebar.update_in(cx, |this, window, cx| {
        this.start_rename(&RowId::from("chat"), window, cx);
    });
}

#[gpui::test]
fn confirming_a_rename_sends_it_and_closes_the_input(cx: &mut TestAppContext) {
    let (recorder, sidebar, cx) = open(cx, DemoData::reference());
    start(&sidebar, cx);
    type_name(&sidebar, "Fresh name", cx);
    sidebar.update(cx, |this, cx| this.finish_edit(EditOutcome::Confirm, cx));
    let expected = Command::Rename {
        row: RowId::from("chat"),
        name: "Fresh name".into(),
    };
    assert_eq!(recorder.sent(), vec![expected]);
    assert!(sidebar.read_with(cx, |this, _| this.inline.is_none()));
}

#[gpui::test]
fn a_refused_name_keeps_the_input_open_with_the_reason(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    start(&sidebar, cx);
    type_name(&sidebar, "main", cx);
    sidebar.update(cx, |this, cx| this.finish_edit(EditOutcome::Confirm, cx));
    let error = sidebar.read_with(cx, |this, _| {
        this.inline.as_ref().and_then(|edit| edit.error.clone())
    });
    assert!(error.is_some_and(|reason| reason.contains("already exists")));
}

#[gpui::test]
fn cancelling_sends_nothing(cx: &mut TestAppContext) {
    let (recorder, sidebar, cx) = open(cx, DemoData::reference());
    start(&sidebar, cx);
    type_name(&sidebar, "ignored", cx);
    sidebar.update(cx, |this, cx| this.finish_edit(EditOutcome::Cancel, cx));
    assert_eq!(recorder.sent(), Vec::<Command>::new());
    assert!(sidebar.read_with(cx, |this, _| this.inline.is_none()));
}

#[gpui::test]
fn branching_sends_make_branch(cx: &mut TestAppContext) {
    let (recorder, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update_in(cx, |this, window, cx| {
        this.start_branching(&RowId::from("chat"), window, cx);
    });
    type_name(&sidebar, "feature", cx);
    sidebar.update(cx, |this, cx| this.finish_edit(EditOutcome::Confirm, cx));
    let expected = Command::MakeBranch {
        row: RowId::from("chat"),
        name: "feature".into(),
    };
    assert_eq!(recorder.sent(), vec![expected]);
}

#[gpui::test]
fn drafting_sends_a_fork(cx: &mut TestAppContext) {
    let (recorder, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update_in(cx, |this, window, cx| {
        this.start_draft(&RowId::from("main"), window, cx);
    });
    type_name(&sidebar, "spike", cx);
    sidebar.update(cx, |this, cx| this.finish_edit(EditOutcome::Confirm, cx));
    let expected = Command::Fork {
        parent: RowId::from("main"),
        name: "spike".into(),
    };
    assert_eq!(recorder.sent(), vec![expected]);
}
