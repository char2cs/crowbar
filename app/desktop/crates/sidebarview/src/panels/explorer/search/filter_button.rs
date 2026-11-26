//! The funnel button beside the Files panel's search box.

use gpui::{
    App, ClickEvent, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{HoverFill, Icon, IconName, Surface, Theme, Tooltip, radius};

use crate::sidebar::ICON_MEDIUM;

const SIZE: f32 = 26.0;

pub(crate) type ClickHandler = Box<dyn Fn(&ClickEvent, &mut Window, &mut App) + 'static>;

#[derive(IntoElement)]
pub(crate) struct FilterButton {
    pub(crate) active: bool,
    pub(crate) on_click: ClickHandler,
}

impl RenderOnce for FilterButton {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let tint = [palette.foreground(), palette.secondary()]
            .get(usize::from(self.active))
            .copied()
            .unwrap_or(palette.foreground());
        let base = div()
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .size(px(SIZE))
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.border())
            .cursor_pointer()
            .child(Icon::new(IconName::Funnel).size(ICON_MEDIUM).color(tint));
        let button = HoverFill::new("file-filter-button", palette.sidebar_element_hover())
            .apply(base, (window, cx))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
            .on_click(self.on_click);
        Tooltip::new("file-filter-tip", "Filter Files", button)
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{TestAppContext, point};

    use super::*;
    use crate::test_support::mount;

    fn counting(clicks: &Rc<Cell<u32>>) -> ClickHandler {
        let clicks = Rc::clone(clicks);
        Box::new(move |_, _, _| clicks.set(clicks.get() + 1))
    }

    #[gpui::test]
    fn a_click_on_the_funnel_runs_the_handler(cx: &mut TestAppContext) {
        let clicks = Rc::new(Cell::new(0));
        let shared = Rc::clone(&clicks);
        let cx = mount(cx, move |_, _| FilterButton {
            active: true,
            on_click: counting(&shared),
        });
        cx.simulate_click(point(px(4.0), px(4.0)), gpui::Modifiers::none());
        assert_eq!(clicks.get(), 1);
    }
}
