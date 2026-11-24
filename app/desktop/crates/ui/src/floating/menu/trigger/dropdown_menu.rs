//! A trigger that opens a [`crate::Menu`] below it.

use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, App, DismissEvent, ElementId, Entity, FocusHandle, InteractiveElement, IntoElement,
    ParentElement, Pixels, Point, RenderOnce, StatefulInteractiveElement, Styled, Window, div,
};

use crate::floating::menu::RowStyle;
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
    over: Option<(Point<Pixels>, Pixels)>,
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
            spec: Spec::new(items.into()),
            placement: Placement::default(),
            over: None,
            trigger_focus: None,
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

impl DropdownMenu {
    pub(crate) fn trigger_focus(
        mut self,
        focus: FocusHandle,
    ) -> Self {
        self.trigger_focus = Some(focus);
        self
    }

    pub(crate) fn options_over(
        mut self,
        chosen: Option<usize>,
        over: (Point<Pixels>, Pixels),
    ) -> Self {
        self.spec.row_style = RowStyle::Option;
        self.spec.highlighted = chosen;
        self.over = Some(over);
        self
    }
}

impl RenderOnce for DropdownMenu {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let given = self.trigger_focus.clone();
        let state =
            window.use_keyed_state((self.id.clone(), "dropdown"), cx, |_, cx| DropdownState {
                open: None,
                trigger: given.unwrap_or_else(|| cx.focus_handle()),
            });
        let open = state.read(cx).open.clone();
        let (trigger_state, spec) = (state.clone(), self.spec);
        let id = self.id.clone();
        let wrapper = div()
            .id(self.id)
            .when_none(&self.trigger_focus, |wrapper| {
                wrapper.track_focus(&state.read(cx).trigger)
            })
            .relative()
            .child(self.trigger)
            .on_click(move |_, window, cx| toggle(&trigger_state, &spec, window, cx));
        let Some(menu) = open else {
            return wrapper.into_any_element();
        };
        let content: popup::Content = Rc::new(move |_, _| menu.clone().into_any_element());
        wrapper
            .child(popup::click_away((id.clone(), "away"), move |_, cx| {
                close(&state, cx);
            }))
            .child(match self.over {
                Some(offset) => popup::over((id, "menu").into(), offset, content),
                None => popup::placed((id, "menu").into(), self.placement, OFFSET, content),
            })
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
    window
        .subscribe(&menu, cx, move |_, _: &DismissEvent, window, cx| {
            let trigger = owner.read(cx).trigger.clone();
            window.focus(&trigger, cx);
            close(&owner, cx);
        })
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
    use crate::test_support::probe::{open, open_floating};

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

    fn one_row() -> Vec<MenuItem> {
        vec![MenuItem::Action(MenuAction::new("Row", |_, _| {}))]
    }

    fn in_dialog(
        closed: &Rc<Cell<usize>>,
        rows: RowStyle,
    ) -> impl Fn(&mut Window, &mut App) -> crate::Dialog + 'static {
        let closed = Rc::clone(closed);
        move |_, _| {
            let closed = Rc::clone(&closed);
            let trigger = div()
                .debug_selector(|| "trigger".to_owned())
                .w(px(120.0))
                .h(px(28.0));
            let menu = DropdownMenu::new("pick", trigger, one_row());
            let menu = match rows {
                RowStyle::Option => menu.options_over(Some(0), (point(px(0.0), px(0.0)), px(0.0))),
                RowStyle::Plain => menu,
            };
            crate::Dialog::new("d")
                .body(menu)
                .on_close(move |_, _| closed.set(closed.get() + 1))
        }
    }

    #[gpui::test]
    fn after_a_choice_escape_reaches_the_dialog_around_a_dropdown_or_select(
        cx: &mut TestAppContext
    ) {
        for rows in [RowStyle::Plain, RowStyle::Option] {
            let closed = Rc::new(Cell::new(0));
            let cx = open(cx, in_dialog(&closed, rows));
            let trigger = cx.debug_bounds("trigger").unwrap_or_default();
            cx.simulate_click(trigger.center(), gpui::Modifiers::none());
            cx.run_until_parked();
            cx.simulate_keystrokes("down enter");
            cx.run_until_parked();
            cx.simulate_keystrokes("escape");
            assert_eq!(closed.get(), 1, "{rows:?}");
        }
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

    #[gpui::test]
    fn an_open_dropdown_floats_in_the_top_layer_where_web_views_cannot_cover_it(
        cx: &mut TestAppContext
    ) {
        let chosen = Rc::new(Cell::new(0));
        let (mut owner, mut top) = open_floating(cx, build(&chosen));
        owner.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
        owner.run_until_parked();
        assert_eq!(owner.debug_bounds("ui-menu"), None, "nothing in the window");
        let menu = top.debug_bounds("ui-menu").unwrap_or_default();
        let trigger = owner.debug_bounds("probe").unwrap_or_default();
        assert!(
            menu.origin.y >= trigger.bottom(),
            "{menu:?} below {trigger:?}"
        );
        top.simulate_keystrokes("down enter");
        assert_eq!(chosen.get(), 1, "the keyboard reaches the floating menu");
    }
}
