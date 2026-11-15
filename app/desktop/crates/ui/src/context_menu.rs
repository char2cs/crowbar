//! A menu opened at the pointer by a right click.

use std::rc::Rc;

use gpui::{
    AnyElement, App, DismissEvent, ElementId, Entity, InteractiveElement, IntoElement, MouseButton,
    ParentElement, Pixels, Point, RenderOnce, Window, div,
};

use crate::context_state::ContextState;
use crate::menu_item::MenuItem;
use crate::menu_spec::MenuSpec;
use crate::opaque_debug::opaque_debug;
use crate::popup;

/// Wraps `child`; a right click on it opens `items` at the pointer, a press outside or `Escape` closes them.
///
/// Rows may open nested menus with `MenuAction::submenu`, and the arrow keys walk them.
#[derive(IntoElement)]
pub struct ContextMenu {
    id: ElementId,
    child: AnyElement,
    spec: MenuSpec,
}

impl ContextMenu {
    /// A context menu over `child` listing `items`.
    pub fn new(
        id: impl Into<ElementId>,
        child: impl IntoElement,
        items: impl Into<Rc<[MenuItem]>>,
    ) -> Self {
        Self {
            id: id.into(),
            child: child.into_any_element(),
            spec: MenuSpec::new(items.into()),
        }
    }

    /// Sets the minimum width of the popup.
    #[must_use]
    pub fn min_width(
        mut self,
        width: Pixels,
    ) -> Self {
        self.spec.min_width = Some(width);
        self
    }
}

impl RenderOnce for ContextMenu {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let state = window.use_keyed_state((self.id.clone(), "context"), cx, |_, _| {
            ContextState::default()
        });
        let (open, at) = {
            let current = state.read(cx);
            (current.open.clone(), current.at)
        };
        let (opener, spec) = (state.clone(), self.spec);
        let wrapper = div().id(self.id).child(self.child).on_mouse_down(
            MouseButton::Right,
            move |event, window, cx| {
                open_at((&opener, &spec), event.position, window, cx);
            },
        );
        let Some(menu) = open else {
            return wrapper.into_any_element();
        };
        wrapper
            .child(popup::click_away(window, move |_, cx| close(&state, cx)))
            .child(popup::at(at, menu))
            .into_any_element()
    }
}

fn open_at(
    (state, spec): (&Entity<ContextState>, &MenuSpec),
    at: Point<Pixels>,
    window: &mut Window,
    cx: &mut App,
) {
    let menu = spec.build(window, cx);
    let owner = state.clone();
    cx.subscribe(&menu, move |_, _: &DismissEvent, cx| close(&owner, cx))
        .detach();
    state.update(cx, |state, cx| {
        state.open = Some(menu);
        state.at = at;
        cx.notify();
    });
}

fn close(
    state: &Entity<ContextState>,
    cx: &mut App,
) {
    state.update(cx, |state, cx| {
        state.open = None;
        cx.notify();
    });
}

opaque_debug!(ContextMenu);

#[cfg(test)]
mod tests {
    use std::cell::Cell;

    use gpui::{Styled, TestAppContext, point, px};

    use super::*;
    use crate::menu_action::MenuAction;
    use crate::test_support::open;

    fn nested(chosen: &Rc<Cell<u32>>) -> Vec<MenuItem> {
        let chosen = Rc::clone(chosen);
        let inner = MenuAction::new("Inner", move |_, _| chosen.set(chosen.get() + 10));
        vec![MenuItem::Action(inner)]
    }

    fn items(chosen: &Rc<Cell<u32>>) -> Vec<MenuItem> {
        let outer = Rc::clone(chosen);
        let rename = MenuAction::new("Rename", move |_, _| outer.set(outer.get() + 1));
        vec![
            MenuItem::Action(rename),
            MenuItem::Action(MenuAction::submenu("Move", nested(chosen))),
        ]
    }

    fn build(chosen: &Rc<Cell<u32>>) -> impl Fn(&mut Window, &mut App) -> ContextMenu + 'static {
        let chosen = Rc::clone(chosen);
        move |_, _| {
            let area = div().w(px(200.0)).h(px(100.0));
            ContextMenu::new("ctx", area, items(&chosen))
        }
    }

    #[gpui::test]
    fn right_click_opens_and_the_arrow_keys_choose(cx: &mut TestAppContext) {
        let chosen = Rc::new(Cell::new(0));
        let cx = open(cx, build(&chosen));
        cx.simulate_mouse_down(
            point(px(10.0), px(10.0)),
            MouseButton::Right,
            gpui::Modifiers::none(),
        );
        cx.run_until_parked();
        cx.simulate_keystrokes("down enter");
        assert_eq!(chosen.get(), 1);
    }

    #[gpui::test]
    fn the_right_arrow_enters_a_submenu_and_left_leaves_it(cx: &mut TestAppContext) {
        let chosen = Rc::new(Cell::new(0));
        let cx = open(cx, build(&chosen));
        cx.simulate_mouse_down(
            point(px(10.0), px(10.0)),
            MouseButton::Right,
            gpui::Modifiers::none(),
        );
        cx.run_until_parked();
        cx.simulate_keystrokes("down down right left right enter");
        assert_eq!(chosen.get(), 10);
    }

    #[gpui::test]
    fn pressing_away_closes_without_choosing(cx: &mut TestAppContext) {
        let chosen = Rc::new(Cell::new(0));
        let cx = open(cx, build(&chosen));
        cx.simulate_mouse_down(
            point(px(10.0), px(10.0)),
            MouseButton::Right,
            gpui::Modifiers::none(),
        );
        cx.run_until_parked();
        cx.simulate_click(point(px(700.0), px(500.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        cx.simulate_keystrokes("enter");
        assert_eq!(chosen.get(), 0);
    }
}
