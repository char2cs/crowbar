use super::*;
use crate::test_support::prelude::*;

use crate::menu_entry::MenuEntry;
use ui::IconName;

fn entries() -> Vec<MenuEntry> {
    vec![
        MenuEntry::action(
            "Rename",
            IconName::PencilSimpleLine,
            Intent::StartRename(RowId::from("chat")),
        ),
        MenuEntry::Separator,
        MenuEntry::action("Keep", IconName::Check, Intent::Keep(RowId::from("chat"))),
    ]
}

fn show(sidebar: &gpui::Entity<Sidebar>, cx: &mut gpui::VisualTestContext) {
    let at = point(px(10.0), px(10.0));
    sidebar.update_in(cx, |this, window, cx| {
        this.open_menu((entries(), at), window, cx);
    });
}

#[gpui::test]
fn an_empty_menu_does_not_open(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    let at = point(px(0.0), px(0.0));
    sidebar.update_in(cx, |this, window, cx| {
        this.open_menu((Vec::new(), at), window, cx);
    });
    assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
}

#[gpui::test]
fn escape_closes_the_menu(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    show(&sidebar, cx);
    sidebar.update_in(cx, |this, window, cx| this.menu_key("escape", window, cx));
    assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
}

#[gpui::test]
fn arrows_skip_separators_and_enter_applies_the_entry(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    show(&sidebar, cx);
    sidebar.update_in(cx, |this, window, cx| {
        this.menu_key("down", window, cx);
        this.menu_key("down", window, cx);
        this.menu_key("enter", window, cx);
    });
    assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
}

#[gpui::test]
fn choosing_an_entry_closes_the_menu_and_applies_its_intent(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    show(&sidebar, cx);
    sidebar.update_in(cx, |this, window, cx| {
        this.choose(Intent::StartRename(RowId::from("chat")), window, cx);
    });
    assert!(sidebar.read_with(cx, |this, _| this.menu.is_none() && this.inline.is_some()));
}

#[gpui::test]
fn highlight_without_a_menu_is_ignored(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update(cx, |this, cx| this.highlight(Some(1), cx));
    assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
}
