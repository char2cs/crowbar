use super::*;
use crate::test_support::prelude::*;

#[gpui::test]
fn the_row_handler_forwards_actions_to_the_sidebar(cx: &mut TestAppContext) {
    let (recorder, sidebar, cx) = open(cx, DemoData::reference());
    let handler = sidebar.update(cx, |_, cx| Sidebar::row_handler(cx));
    cx.update(|window, cx| handler(RowAction::Open(RowId::from("main")), window, cx));
    cx.run_until_parked();
    assert_eq!(recorder.sent(), vec![Command::OpenRow(RowId::from("main"))]);
}

#[gpui::test]
fn selecting_a_space_makes_it_the_active_one(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::rich());
    let other = sidebar.read_with(cx, |this, _| {
        let active = this.data.active_project();
        this.data
            .projects()
            .into_iter()
            .map(|project| project.id)
            .find(|id| Some(id) != active.as_ref())
    });
    let other = other.expect("a second space");
    sidebar.update(cx, |this, cx| {
        this.send(Command::SelectProject(other.clone()), cx);
    });
    cx.run_until_parked();
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.data.active_project()),
        Some(other)
    );
}

#[gpui::test]
fn the_loading_state_renders_a_skeleton_instead_of_rows(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::loading());
    cx.run_until_parked();
    assert_eq!(sidebar.read_with(cx, |this, _| this.mode), Mode::Chats);
}
