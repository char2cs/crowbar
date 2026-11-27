use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{HoverFill, Icon, IconName, Surface, Theme, Tooltip, TooltipSide, radius};

use crate::tabbar::tab_actions::TabAction;
use crate::toprow::row_ink::RowInk;

const SIZE: f32 = 28.0;
const GLYPH: f32 = 16.0;
#[derive(IntoElement)]
pub(crate) struct RowButton {
    pub(crate) id: SharedString,
    pub(crate) icon: IconName,
    pub(crate) tip: SharedString,
    pub(crate) ink: RowInk,
    pub(crate) on_click: TabAction,
}

impl RenderOnce for RowButton {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let glyph = Icon::new(self.icon)
            .size(px(GLYPH))
            .color(self.ink.color(palette));
        let base = div()
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .size(px(SIZE))
            .rounded(radius::SM)
            .cursor_pointer()
            .debug_selector({
                let id = self.id.clone();
                move || format!("row-button-{id}")
            })
            .child(glyph);
        let click = self.on_click;
        let button = HoverFill::new(
            ElementId::from(self.id.clone()),
            palette.sidebar_element_hover(),
        )
        .apply(base, (window, cx))
        .on_mouse_down(MouseButton::Left, |_, window, cx| {
            window.prevent_default();
            cx.stop_propagation();
        })
        .on_click(move |_, window, cx| click(window, cx));
        let tip_id = ElementId::from(SharedString::from(format!("{}-tip", self.id)));
        Tooltip::new(tip_id, self.tip, button).side(TooltipSide::Bottom)
    }
}

ui::opaque_debug!(RowButton);

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Context, Modifiers, Render, TestAppContext};
    use ui::Motion;

    use super::*;

    struct Shown {
        clicks: Rc<Cell<u32>>,
    }

    impl Render for Shown {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let clicks = Rc::clone(&self.clicks);
            div().child(RowButton {
                id: "split".into(),
                icon: IconName::SquareSplitHorizontal,
                tip: "Show editor alongside chat".into(),
                ink: RowInk::Muted,
                on_click: Rc::new(move |_, _| clicks.set(clicks.get() + 1)),
            })
        }
    }

    #[gpui::test]
    fn a_click_runs_the_action_once(cx: &mut TestAppContext) {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let clicks = Rc::new(Cell::new(0));
        let counted = Rc::clone(&clicks);
        let (_, cx) = cx.add_window_view(move |_, _| Shown { clicks: counted });
        cx.run_until_parked();
        let at = cx.debug_bounds("row-button-split").unwrap_or_default();
        assert_eq!(at.size.width, px(SIZE), "square, develop's icon-sm");
        cx.simulate_click(at.center(), Modifiers::none());
        assert_eq!(clicks.get(), 1);
    }
}
