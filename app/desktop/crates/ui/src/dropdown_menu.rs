//! A trigger that opens a [`Menu`] below it.

use std::rc::Rc;

use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce, StatefulInteractiveElement,
    Styled, Subscription, Window, div,
};

use crate::dropdown_state::DropdownState;
use crate::menu::Menu;
use crate::menu_item::MenuItem;
use crate::popup;

const OFFSET: f32 = 4.0;

/// Wraps a trigger; clicking it opens the menu, any press outside or `Escape` closes it.
#[derive(IntoElement)]
pub struct DropdownMenu {
    id: ElementId,
    trigger: AnyElement,
    items: Rc<[MenuItem]>,
    full_width: bool,
    min_width: Option<gpui::Pixels>,
}

impl DropdownMenu {
    /// A dropdown for `trigger` listing `items`.
    pub fn new(id: impl Into<ElementId>, trigger: impl IntoElement, items: impl Into<Rc<[MenuItem]>>) -> Self {
        Self { id: id.into(), trigger: trigger.into_any_element(), items: items.into(), full_width: false, min_width: None }
    }

    /// Sets the minimum width of the popup.
    #[must_use]
    pub fn min_width(mut self, width: gpui::Pixels) -> Self {
        self.min_width = Some(width);
        self
    }

    /// Makes the wrapper fill its row, so the menu is as wide as the trigger.
    #[must_use]
    pub fn full_width(mut self, full_width: bool) -> Self {
        self.full_width = full_width;
        self
    }
}

impl RenderOnce for DropdownMenu {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let state = window.use_keyed_state((self.id.clone(), "dropdown"), cx, |_, _| DropdownState::default());
        let open = state.read(cx).open.clone();
        let toggle_state = state.clone();
        let items = Rc::clone(&self.items);
        let min_width = self.min_width;
        let wrapper = div()
            .id(self.id)
            .relative()
            .when_width(self.full_width)
            .child(self.trigger)
            .on_click(move |_, window, cx| {
                if toggle_state.read(cx).open.is_some() {
                    return;
                }
                open_menu(&toggle_state, Rc::clone(&items), min_width, window, cx);
            });
        match open {
            None => wrapper.into_any_element(),
            Some(menu) => {
                let close = state.clone();
                wrapper
                    .child(popup::click_away(window, move |_, cx| close_menu(&close, cx)))
                    .child(popup::below(OFFSET, menu))
                    .into_any_element()
            }
        }
    }
}

trait WidthExt: Styled + Sized {
    fn when_width(self, full: bool) -> Self {
        if full { self.w_full() } else { self }
    }
}

impl WidthExt for gpui::Stateful<gpui::Div> {}

fn open_menu(
    state: &gpui::Entity<DropdownState>,
    items: Rc<[MenuItem]>,
    min_width: Option<gpui::Pixels>,
    window: &mut Window,
    cx: &mut App,
) {
    let menu = cx.new(|cx| {
        let menu = Menu::new(items, window, cx);
        match min_width {
            Some(width) => menu.min_width(width),
            None => menu,
        }
    });
    let owner = state.clone();
    let subscription: Subscription = cx.subscribe(&menu, move |_, _: &gpui::DismissEvent, cx| close_menu(&owner, cx));
    subscription.detach();
    state.update(cx, |state, cx| {
        state.open = Some(menu);
        cx.notify();
    });
}

fn close_menu(state: &gpui::Entity<DropdownState>, cx: &mut App) {
    state.update(cx, |state, cx| {
        state.open = None;
        cx.notify();
    });
}

#[cfg(test)]
#[path = "dropdown_menu_tests.rs"]
mod tests;
