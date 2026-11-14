use super::*;
use crate::test_support::prelude::*;

#[gpui::test]
fn opening_a_row_sends_the_open_command(cx: &mut TestAppContext) {
    let (recorder, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update_in(cx, |this, window, cx| {
        this.act(RowAction::Open(RowId::from("chat")), window, cx);
    });
    assert_eq!(recorder.sent(), vec![Command::OpenRow(RowId::from("chat"))]);
}

#[gpui::test]
fn the_row_menu_opens_at_the_pointer(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    let at = point(px(40.0), px(200.0));
    sidebar.update_in(cx, |this, window, cx| {
        this.act(
            RowAction::Menu {
                row: RowId::from("chat"),
                at,
            },
            window,
            cx,
        );
    });
    let menu_at = sidebar.read_with(cx, |this, _| this.menu.as_ref().map(|menu| menu.at));
    assert_eq!(menu_at, Some(at));
}

#[gpui::test]
fn a_menu_for_a_missing_row_stays_closed(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    let at = point(px(0.0), px(0.0));
    sidebar.update_in(cx, |this, window, cx| {
        this.act(
            RowAction::Menu {
                row: RowId::from("gone"),
                at,
            },
            window,
            cx,
        );
    });
    assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
}

#[gpui::test]
fn toggling_a_fold_hides_the_children_and_is_remembered(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update_in(cx, |this, window, cx| {
        this.apply(Intent::ToggleFold(RowId::from("demo")), window, cx);
    });
    let folded = sidebar.read_with(cx, |this, _| this.tree.folded().count());
    assert_eq!(folded, 1);
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.preferences().folded.len()),
        1
    );
}

#[gpui::test]
fn a_refused_command_leaves_the_sidebar_unchanged(cx: &mut TestAppContext) {
    let (recorder, sidebar, cx) = open(cx, DemoData::reference());
    let command = Command::Rename {
        row: RowId::from("chat"),
        name: "main".into(),
    };
    sidebar.update(cx, |this, cx| this.send(command.clone(), cx));
    assert_eq!(recorder.sent(), vec![command]);
}
