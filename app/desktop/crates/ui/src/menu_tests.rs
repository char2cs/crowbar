use std::cell::RefCell;
use std::rc::Rc;

use gpui::{DismissEvent, Entity, TestAppContext, VisualTestContext};

use super::*;
use crate::menu_action::MenuAction;
use crate::menu_actions;
use crate::motion::Motion;

type Log = Rc<RefCell<Vec<&'static str>>>;

fn items(log: &Log) -> Vec<MenuItem> {
    let record = |name: &'static str| {
        let log = Rc::clone(log);
        move |_: &mut Window, _: &mut App| log.borrow_mut().push(name)
    };
    vec![
        MenuItem::Action(MenuAction::new("First", record("first"))),
        MenuItem::Action(MenuAction::new("Off", record("off")).disabled()),
        MenuItem::Separator,
        MenuItem::Action(MenuAction::new("Last", record("last"))),
    ]
}

type Opened<'a> = (Entity<Menu>, &'a mut VisualTestContext, Rc<RefCell<u32>>);

fn open_menu<'a>(cx: &'a mut TestAppContext, log: &Log) -> Opened<'a> {
    cx.update(|cx| {
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
        cx.bind_keys(menu_actions::bindings());
    });
    let entries = items(log);
    let (menu, cx) = cx.add_window_view(move |window, cx| Menu::new(entries, window, cx));
    let dismissed = Rc::new(RefCell::new(0));
    let counter = Rc::clone(&dismissed);
    cx.update(|_, cx| {
        cx.subscribe(&menu, move |_, _: &DismissEvent, _| {
            *counter.borrow_mut() += 1;
        })
        .detach();
    });
    (menu, cx, dismissed)
}

#[gpui::test]
fn arrow_keys_skip_disabled_rows_and_enter_runs_the_highlighted_one(cx: &mut TestAppContext) {
    let log = Log::default();
    let (_, cx, dismissed) = open_menu(cx, &log);
    cx.simulate_keystrokes("down down enter");
    assert_eq!(*log.borrow(), ["last"]);
    assert_eq!(*dismissed.borrow(), 1);
}

#[gpui::test]
fn up_from_nothing_selects_the_last_row(cx: &mut TestAppContext) {
    let log = Log::default();
    let (_, cx, _) = open_menu(cx, &log);
    cx.simulate_keystrokes("up enter");
    assert_eq!(*log.borrow(), ["last"]);
}

#[gpui::test]
fn enter_without_a_highlight_does_nothing(cx: &mut TestAppContext) {
    let log = Log::default();
    let (_, cx, dismissed) = open_menu(cx, &log);
    cx.simulate_keystrokes("enter");
    assert!(log.borrow().is_empty());
    assert_eq!(*dismissed.borrow(), 0);
}

#[gpui::test]
fn escape_dismisses_without_running_anything(cx: &mut TestAppContext) {
    let log = Log::default();
    let (_, cx, dismissed) = open_menu(cx, &log);
    cx.simulate_keystrokes("down escape");
    assert!(log.borrow().is_empty());
    assert_eq!(*dismissed.borrow(), 1);
}
