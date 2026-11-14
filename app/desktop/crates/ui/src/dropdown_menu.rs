//! A trigger that opens a [`crate::Menu`] below it.

use std::rc::Rc;

use gpui::{
    AnyElement, App, DismissEvent, ElementId, Entity, InteractiveElement, IntoElement,
    ParentElement, Pixels, RenderOnce, StatefulInteractiveElement, Styled, Window, div,
};

use crate::dropdown_state::DropdownState;
use crate::menu_item::MenuItem;
use crate::menu_spec::MenuSpec;
use crate::opaque_debug::opaque_debug;
use crate::popup;

const OFFSET: f32 = 4.0;

/// Wraps a trigger; clicking it opens the menu, any press outside or `Escape` closes it.
#[derive(IntoElement)]
pub struct DropdownMenu {
    id: ElementId,
    trigger: AnyElement,
    spec: MenuSpec,
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
            spec: MenuSpec {
                items: items.into(),
                min_width: None,
            },
        }
    }

    /// Sets the minimum width of the popup.
    #[must_use]
    pub fn min_width(mut self, width: Pixels) -> Self {
        self.spec.min_width = Some(width);
        self
    }
}

impl RenderOnce for DropdownMenu {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
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
            .child(popup::below(OFFSET, menu))
            .into_any_element()
    }
}

fn toggle(state: &Entity<DropdownState>, spec: &MenuSpec, window: &mut Window, cx: &mut App) {
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

fn close(state: &Entity<DropdownState>, cx: &mut App) {
    state.update(cx, |state, cx| {
        state.open = None;
        cx.notify();
    });
}

opaque_debug!(DropdownMenu);

#[cfg(test)]
#[path = "dropdown_menu_tests.rs"]
mod tests;
