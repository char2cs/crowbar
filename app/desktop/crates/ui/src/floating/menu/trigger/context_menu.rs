//! A menu opened at the pointer by a right click.

use std::rc::Rc;

use gpui::{
    AnyElement, App, DismissEvent, Div, ElementId, Entity, InteractiveElement, IntoElement,
    MouseButton, ParentElement, Pixels, Point, Refineable as _, RenderOnce, Stateful,
    StyleRefinement, Styled, Window, div, px,
};

use crate::floating::menu::menu_item::MenuItem;
use crate::floating::menu::native_menu::{self, MenuEdge, NativeMenu};
use crate::floating::menu::trigger::context_state::ContextState;
use crate::floating::menu::trigger::spec::Spec;
use crate::floating::placement::Placement;
use crate::floating::popup;
use crate::foundation::opaque_debug;

const OFFSET: f32 = 4.0;

/// Wraps `child`; a right click on it opens `items` at the pointer, as the installed
/// [`NativeMenu`](crate::NativeMenu) when there is one, else drawn in the window, where a press
/// outside or `Escape` closes them.
///
/// Rows may open nested menus with `MenuAction::submenu`, and the arrow keys walk them.
#[derive(IntoElement)]
pub struct ContextMenu {
    id: ElementId,
    child: AnyElement,
    spec: Spec,
    placement: Option<Placement>,
    style: StyleRefinement,
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
            spec: Spec::new(items.into()),
            placement: None,
            style: StyleRefinement::default(),
        }
    }

    /// Opens the menu at `placement` of the wrapped element instead of at the pointer; a native
    /// menu starts there, as develop pops its tab menu under the tab's centre.
    #[must_use]
    pub fn placement(
        mut self,
        placement: Placement,
    ) -> Self {
        self.placement = Some(placement);
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

/// Styles the area that takes the right click, so it can grow to fill free space.
impl Styled for ContextMenu {
    fn style(&mut self) -> &mut StyleRefinement {
        &mut self.style
    }
}

impl RenderOnce for ContextMenu {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        match NativeMenu::installed(cx) {
            Some(native) => self.native(native),
            None => self.drawn(window, cx),
        }
    }
}

impl ContextMenu {
    fn area(&mut self) -> Stateful<Div> {
        let child = std::mem::replace(&mut self.child, div().into_any_element());
        let mut area = div().id(self.id.clone()).relative();
        area.style().refine(&self.style);
        area.child(child)
    }

    fn native(
        mut self,
        native: NativeMenu,
    ) -> AnyElement {
        let (items, placement) = (Rc::clone(&self.spec.items), self.placement);
        let (laid_out, probe) = native_menu::trigger_probe();
        // PLATFORM: linux the press holds an implicit X grab on the shell window, so GTK can only take the pointer for its menu once the button is up.
        self.area()
            .child(probe)
            .on_mouse_up(MouseButton::Right, move |event, window, cx| {
                let pointer = (event.position, MenuEdge::Start);
                let at = placement.map_or(pointer, |side| side.beside(laid_out.get(), px(OFFSET)));
                native.open(&items, at, (window, cx));
            })
            .into_any_element()
    }

    fn drawn(
        mut self,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        let area = self.area();
        let id = self.id;
        let state =
            window.use_keyed_state((id.clone(), "context"), cx, |_, _| ContextState::default());
        let (open, at) = {
            let current = state.read(cx);
            (current.open.clone(), current.at)
        };
        let (opener, spec) = (state.clone(), self.spec);
        let wrapper = area.on_mouse_down(MouseButton::Right, move |event, window, cx| {
            spec.open_at(&opener, event.position, (window, cx));
        });
        let Some(menu) = open else {
            return wrapper.into_any_element();
        };
        let content: popup::Content = Rc::new(move |_, _| menu.clone().into_any_element());
        let panel = match self.placement {
            Some(placement) => {
                popup::placed((id.clone(), "menu").into(), placement, OFFSET, content)
            },
            None => popup::popup_at((id.clone(), "menu"), at, move |window, cx| {
                content(window, cx)
            }),
        };
        wrapper
            .child(popup::click_away((id, "away"), move |_, cx| {
                close(&state, cx);
            }))
            .child(panel)
            .into_any_element()
    }
}

impl Spec {
    fn open_at(
        &self,
        state: &Entity<ContextState>,
        at: Point<Pixels>,
        (window, cx): (&mut Window, &mut App),
    ) {
        let menu = self.build(window, cx);
        let owner = state.clone();
        cx.subscribe(&menu, move |_, _: &DismissEvent, cx| close(&owner, cx))
            .detach();
        state.update(cx, |state, cx| {
            state.open = Some(menu);
            state.at = at;
            cx.notify();
        });
    }
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
    use crate::floating::menu::menu_item::MenuAction;
    use crate::test_support::probe::open;

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
    fn a_placed_menu_opens_under_the_centre_of_its_child_wherever_the_pointer_is(
        cx: &mut TestAppContext
    ) {
        let cx = open(cx, |_, _| {
            let area = div().w(px(200.0)).h(px(100.0));
            let rows = vec![MenuItem::Action(MenuAction::new("Row", |_, _| {}))];
            ContextMenu::new("placed", area, rows).placement(Placement::BelowCentre)
        });
        cx.simulate_mouse_down(
            point(px(10.0), px(10.0)),
            MouseButton::Right,
            gpui::Modifiers::none(),
        );
        cx.run_until_parked();
        let area = cx.debug_bounds("probe").unwrap_or_default();
        let menu = cx.debug_bounds("ui-menu").unwrap_or_default();
        assert!(
            (menu.origin.x - (area.origin.x + px(100.0))).abs() < px(0.5),
            "{menu:?} in {area:?}"
        );
        assert!(
            (menu.origin.y - (area.bottom() + px(4.0))).abs() < px(0.5),
            "{menu:?} in {area:?}"
        );
    }

    #[gpui::test]
    fn a_menu_styled_to_grow_takes_a_right_click_anywhere_in_the_space_it_fills(
        cx: &mut TestAppContext
    ) {
        let cx = open(cx, |_, _| {
            let rows = vec![MenuItem::Action(MenuAction::new("Row", |_, _| {}))];
            let foot = ContextMenu::new("foot", div(), rows).flex_1();
            div()
                .flex()
                .flex_col()
                .w(px(200.0))
                .h(px(300.0))
                .child(div().h(px(20.0)))
                .child(foot)
        });
        let low = point(px(10.0), px(280.0));
        cx.simulate_mouse_down(low, MouseButton::Right, gpui::Modifiers::none());
        cx.run_until_parked();
        assert!(cx.debug_bounds("ui-menu").is_some());
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
