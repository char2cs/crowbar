//! A trigger that opens a menu below it.

use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, ElementId, FocusHandle, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::floating::menu::menu_item::MenuItem;
use crate::floating::menu::native_menu::{self, NativeMenu};
use crate::floating::placement::Placement;
use crate::foundation::opaque_debug;

const OFFSET: f32 = 4.0;

/// Wraps a trigger; clicking it opens `items` as the desktop's menu.
#[derive(IntoElement)]
pub struct DropdownMenu {
    id: ElementId,
    trigger: gpui::AnyElement,
    items: Rc<[MenuItem]>,
    placement: Placement,
    trigger_focus: Option<FocusHandle>,
}

impl DropdownMenu {
    /// A dropdown for `trigger` listing `items`.
    pub fn new(
        id: impl Into<ElementId>,
        trigger: impl IntoElement,
        items: impl Into<Rc<[MenuItem]>>,
    ) -> Self {
        Self {
            id: id.into(),
            trigger: trigger.into_any_element(),
            items: items.into(),
            placement: Placement::default(),
            trigger_focus: None,
        }
    }

    /// Sets where the menu opens relative to the trigger; under it, left edges aligned, by default.
    #[must_use]
    pub fn placement(
        mut self,
        placement: Placement,
    ) -> Self {
        self.placement = placement;
        self
    }

    pub(crate) fn trigger_focus(
        mut self,
        focus: FocusHandle,
    ) -> Self {
        self.trigger_focus = Some(focus);
        self
    }
}

impl RenderOnce for DropdownMenu {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let focus =
            window.use_keyed_state((self.id.clone(), "dropdown"), cx, |_, cx| cx.focus_handle());
        let focus = focus.read(cx).clone();
        let (items, placement) = (self.items, self.placement);
        let (laid_out, probe) = native_menu::trigger_probe();
        div()
            .id(self.id)
            .when_none(&self.trigger_focus, |wrapper| wrapper.track_focus(&focus))
            .relative()
            .child(self.trigger)
            .child(probe)
            .on_click(move |_, window, cx| {
                let at = placement.beside(laid_out.get(), px(OFFSET));
                NativeMenu::open(&items, at, (window, cx));
            })
    }
}

opaque_debug!(DropdownMenu);

#[cfg(test)]
mod tests {
    use std::cell::Cell;

    use gpui::{Modifiers, TestAppContext, point};

    use super::*;
    use crate::control::Button;
    use crate::floating::menu::menu_item::MenuAction;
    use crate::floating::menu::native_menu::{MenuEdge, NativeEntry};
    use crate::test_support::menus::answering;
    use crate::test_support::probe::open;

    fn one_row() -> Vec<MenuItem> {
        vec![MenuItem::Action(MenuAction::new("Row", |_, _| {}))]
    }

    fn build(chosen: &Rc<Cell<usize>>) -> impl Fn(&mut Window, &mut App) -> DropdownMenu + 'static {
        let chosen = Rc::clone(chosen);
        move |_, _| {
            let chosen = Rc::clone(&chosen);
            let rename = MenuAction::new("Rename", move |_, _| chosen.set(chosen.get() + 1));
            let items = vec![MenuItem::Action(rename)];
            DropdownMenu::new("dd", Button::new("trigger", "Open"), items)
        }
    }

    #[gpui::test]
    fn a_dropdown_asks_for_its_menu_under_its_trigger_and_runs_the_choice(cx: &mut TestAppContext) {
        let chosen = Rc::new(Cell::new(0));
        let asked = answering(cx, Some(vec![0]));
        let cx = open(cx, build(&chosen));
        cx.simulate_click(point(px(10.0), px(10.0)), Modifiers::none());
        cx.run_until_parked();
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let opened_at = asked
            .borrow()
            .first()
            .map(|request| (request.at, request.edge));
        let under = point(trigger.left(), trigger.bottom() + px(4.0));
        assert_eq!(opened_at, Some((under, MenuEdge::Start)));
        assert_eq!(chosen.get(), 1);
    }

    #[gpui::test]
    fn an_end_placed_dropdown_ends_with_its_trigger(cx: &mut TestAppContext) {
        let asked = answering(cx, None);
        let cx = open(cx, |_, _| {
            let trigger = div().w(px(300.0)).h(px(20.0));
            DropdownMenu::new("end", trigger, one_row()).placement(Placement::BelowEnd)
        });
        cx.simulate_click(point(px(10.0), px(10.0)), Modifiers::none());
        cx.run_until_parked();
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let opened_at = asked
            .borrow()
            .first()
            .map(|request| (request.at, request.edge));
        let under = point(trigger.right(), trigger.bottom() + px(4.0));
        assert_eq!(opened_at, Some((under, MenuEdge::End)));
    }

    #[gpui::test]
    fn after_a_choice_escape_reaches_the_dialog_around_the_dropdown(cx: &mut TestAppContext) {
        answering(cx, Some(vec![0]));
        let closed = Rc::new(Cell::new(0));
        let heard = Rc::clone(&closed);
        let cx = open(cx, move |_, _| {
            let heard = Rc::clone(&heard);
            let trigger = div()
                .debug_selector(|| "trigger".to_owned())
                .w(px(120.0))
                .h(px(28.0));
            crate::Dialog::new("d")
                .body(DropdownMenu::new("pick", trigger, one_row()))
                .on_close(move |_, _| heard.set(heard.get() + 1))
        });
        let trigger = cx.debug_bounds("trigger").unwrap_or_default();
        cx.simulate_click(trigger.center(), Modifiers::none());
        cx.run_until_parked();
        cx.simulate_keystrokes("escape");
        assert_eq!(closed.get(), 1);
    }

    #[gpui::test]
    fn a_select_checks_its_chosen_option(cx: &mut TestAppContext) {
        let asked = answering(cx, None);
        let cx = open(cx, |_, _| {
            crate::Select::new("s", vec!["System".into(), "Dark".into()], Some(1))
        });
        cx.simulate_click(point(px(10.0), px(10.0)), Modifiers::none());
        cx.run_until_parked();
        let checked: Vec<bool> = asked
            .borrow()
            .iter()
            .flat_map(|request| request.entries.iter())
            .map(|entry| matches!(entry, NativeEntry::Item { checked: true, .. }))
            .collect();
        assert_eq!(checked, [false, true]);
    }
}
