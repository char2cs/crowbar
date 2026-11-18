//! A trigger that opens a [`crate::Menu`] below it.

use std::rc::Rc;

use gpui::{
    AnyElement, App, DismissEvent, ElementId, Entity, InteractiveElement, IntoElement,
    ParentElement, Pixels, RenderOnce, StatefulInteractiveElement, Styled, Window, div,
};

use crate::floating::menu::menu_item::MenuItem;
use crate::floating::menu::trigger::dropdown_state::DropdownState;
use crate::floating::menu::trigger::spec::Spec;
use crate::floating::placement::Placement;
use crate::floating::popup;
use crate::foundation::opaque_debug;

const OFFSET: f32 = 4.0;

/// Wraps a trigger; clicking it opens the menu, any press outside or `Escape` closes it.
#[derive(IntoElement)]
pub struct DropdownMenu {
    id: ElementId,
    trigger: AnyElement,
    spec: Spec,
    placement: Placement,
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
            spec: Spec::new(items.into()),
            placement: Placement::default(),
        }
    }

    /// Sets where the popup opens relative to the trigger; under it, left edges aligned, by default.
    #[must_use]
    pub fn placement(
        mut self,
        placement: Placement,
    ) -> Self {
        self.placement = placement;
        self
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

impl RenderOnce for DropdownMenu {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let state = window.use_keyed_state((self.id.clone(), "dropdown"), cx, |_, _| {
            DropdownState::default()
        });
        let open = state.read(cx).open.clone();
        let (trigger_state, spec) = (state.clone(), self.spec);
        let wrapper = div()
            .id(self.id)
            .relative()
            .child(self.trigger)
            .on_click(move |_, window, cx| toggle(&trigger_state, &spec, window, cx));
        let Some(menu) = open else {
            return wrapper.into_any_element();
        };
        wrapper
            .child(popup::click_away(window, move |_, cx| close(&state, cx)))
            .child(popup::placed(self.placement, OFFSET, menu))
            .into_any_element()
    }
}

fn toggle(
    state: &Entity<DropdownState>,
    spec: &Spec,
    window: &mut Window,
    cx: &mut App,
) {
    if state.read(cx).open.is_some() {
        return;
    }
    let menu = spec.build(window, cx);
    let owner = state.clone();
    cx.subscribe(&menu, move |_, _: &DismissEvent, cx| close(&owner, cx))
        .detach();
    state.update(cx, |state, cx| {
        state.open = Some(menu);
        cx.notify();
    });
}

fn close(
    state: &Entity<DropdownState>,
    cx: &mut App,
) {
    state.update(cx, |state, cx| {
        state.open = None;
        cx.notify();
    });
}

opaque_debug!(DropdownMenu);

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{TestAppContext, point, px};

    use super::*;
    use crate::control::Button;
    use crate::floating::menu::menu_item::MenuAction;
    use crate::test_support::probe::open;

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
    fn clicking_the_trigger_opens_the_menu_and_clicking_away_closes_it(cx: &mut TestAppContext) {
        let chosen = Rc::new(Cell::new(0));
        let cx = open(cx, build(&chosen));
        cx.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        cx.simulate_click(point(px(700.0), px(500.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        assert!(
            chosen.get() == 0,
            "closing by clicking away does not choose a row"
        );
    }

    #[gpui::test]
    fn keyboard_choice_runs_the_row_and_closes(cx: &mut TestAppContext) {
        let chosen = Rc::new(Cell::new(0));
        let cx = open(cx, build(&chosen));
        cx.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        cx.simulate_keystrokes("down enter");
        assert_eq!(chosen.get(), 1);
    }

    #[gpui::test]
    fn an_end_placed_menu_shares_the_right_edge_of_its_trigger(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            let trigger = div().w(px(300.0)).h(px(20.0));
            let rows = vec![MenuItem::Action(MenuAction::new("Row", |_, _| {}))];
            DropdownMenu::new("end", trigger, rows).placement(Placement::BelowEnd)
        });
        cx.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let menu = cx.debug_bounds("ui-menu").unwrap_or_default();
        assert!(
            (menu.right() - trigger.right()).abs() < px(0.5),
            "{menu:?} ends with {trigger:?}"
        );
    }
}
