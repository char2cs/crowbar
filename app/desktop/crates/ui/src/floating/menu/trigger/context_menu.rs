//! A menu opened by a right click.

use std::rc::Rc;

use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    Refineable as _, RenderOnce, StyleRefinement, Styled, Window, div, px,
};

use crate::floating::menu::menu_item::MenuItem;
use crate::floating::menu::native_menu::{self, MenuEdge, NativeMenu};
use crate::floating::placement::Placement;
use crate::foundation::opaque_debug;

const OFFSET: f32 = 4.0;

/// Wraps `child`; a right click on it opens `items` as the desktop's menu, at the pointer.
#[derive(IntoElement)]
pub struct ContextMenu {
    id: ElementId,
    child: AnyElement,
    items: Rc<[MenuItem]>,
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
            items: items.into(),
            placement: None,
            style: StyleRefinement::default(),
        }
    }

    /// Opens the menu at `placement` of the wrapped element instead of at the pointer, as develop
    /// pops its tab menu under the tab's centre.
    #[must_use]
    pub fn placement(
        mut self,
        placement: Placement,
    ) -> Self {
        self.placement = Some(placement);
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
        _window: &mut Window,
        _cx: &mut App,
    ) -> impl IntoElement {
        let (items, placement) = (self.items, self.placement);
        let (laid_out, probe) = native_menu::trigger_probe();
        let mut area = div().id(self.id).relative();
        area.style().refine(&self.style);
        // PLATFORM: linux the press holds an implicit X grab on the shell window, so GTK can only take the pointer for its menu once the button is up.
        area.child(self.child).child(probe).on_mouse_up(
            MouseButton::Right,
            move |event, window, cx| {
                let pointer = (event.position, MenuEdge::Start);
                let at = placement.map_or(pointer, |side| side.beside(laid_out.get(), px(OFFSET)));
                NativeMenu::open(&items, at, (window, cx));
            },
        )
    }
}

opaque_debug!(ContextMenu);

#[cfg(test)]
mod tests {
    use gpui::{Modifiers, TestAppContext, point};

    use super::*;
    use crate::floating::menu::menu_item::MenuAction;
    use crate::test_support::menus::answering;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn a_menu_styled_to_grow_takes_a_right_click_anywhere_in_the_space_it_fills(
        cx: &mut TestAppContext
    ) {
        let asked = answering(cx, None);
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
        cx.simulate_mouse_down(low, MouseButton::Right, Modifiers::none());
        cx.simulate_mouse_up(low, MouseButton::Right, Modifiers::none());
        cx.run_until_parked();
        assert_eq!(asked.borrow().len(), 1);
    }
}
