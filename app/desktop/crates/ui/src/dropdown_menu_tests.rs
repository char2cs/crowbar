use std::cell::Cell;
use std::rc::Rc;

use gpui::{TestAppContext, point, px};

use super::*;
use crate::button::Button;
use crate::menu_action::MenuAction;
use crate::test_support::open;

fn build(chosen: &Rc<Cell<bool>>) -> impl Fn(&mut Window, &mut App) -> DropdownMenu + 'static {
    let chosen = Rc::clone(chosen);
    move |_, _| {
        let chosen = Rc::clone(&chosen);
        let items = vec![MenuItem::Action(MenuAction::new("Rename", move |_, _| {
            chosen.set(true);
        }))];
        DropdownMenu::new("dd", Button::new("trigger", "Open"), items)
    }
}

#[gpui::test]
fn clicking_the_trigger_opens_the_menu_and_clicking_away_closes_it(cx: &mut TestAppContext) {
    let chosen = Rc::new(Cell::new(false));
    let cx = open(cx, build(&chosen));
    cx.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
    cx.run_until_parked();
    cx.simulate_click(point(px(700.0), px(500.0)), gpui::Modifiers::none());
    cx.run_until_parked();
    assert!(
        !chosen.get(),
        "closing by clicking away does not choose a row"
    );
}

#[gpui::test]
fn keyboard_choice_runs_the_row_and_closes(cx: &mut TestAppContext) {
    let chosen = Rc::new(Cell::new(false));
    let cx = open(cx, build(&chosen));
    cx.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
    cx.run_until_parked();
    cx.simulate_keystrokes("down enter");
    assert!(chosen.get());
}
