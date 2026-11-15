//! The square ghost buttons of the sidebar header and switcher.

use gpui::{
    App, ClickEvent, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    Pixels, RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Icon, IconName, Surface, Theme, Tooltip, hover_fill, radius};

use crate::tile_shape::TileShape;

const SIZE: f32 = 28.0;
const ICON: f32 = 16.0;

type ClickHandler = Box<dyn Fn(&ClickEvent, &mut Window, &mut App) + 'static>;

#[derive(IntoElement)]
pub(crate) struct IconButton {
    id: ElementId,
    icon: IconName,
    tooltip: Option<SharedString>,
    shape: TileShape,
    on_click: Option<ClickHandler>,
}

impl IconButton {
    pub(crate) fn new(
        id: impl Into<ElementId>,
        icon: IconName,
    ) -> Self {
        Self {
            id: id.into(),
            icon,
            tooltip: None,
            shape: TileShape::Small,
            on_click: None,
        }
    }

    pub(crate) fn tooltip(
        mut self,
        text: impl Into<SharedString>,
    ) -> Self {
        self.tooltip = Some(text.into());
        self
    }

    pub(crate) fn shape(
        mut self,
        shape: TileShape,
    ) -> Self {
        self.shape = shape;
        self
    }

    pub(crate) fn on_click(
        mut self,
        handler: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_click = Some(Box::new(handler));
        self
    }

    fn radius(&self) -> Pixels {
        match self.shape {
            TileShape::Small => radius::SM,
            TileShape::Tile => radius::MD,
        }
    }
}

impl RenderOnce for IconButton {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let radius = self.radius();
        let base = div()
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .size(px(SIZE))
            .rounded(radius)
            .cursor_pointer()
            .child(
                Icon::new(self.icon)
                    .size(px(ICON))
                    .color(palette.muted_foreground()),
            );
        let id = self.id.clone();
        let button = hover_fill(base, self.id, palette.sidebar_element_hover(), (window, cx))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default());
        let button = match self.on_click {
            Some(handler) => button.on_click(handler),
            None => button,
        };
        match self.tooltip {
            Some(text) => Tooltip::new((id, "tip"), text, button).into_any_element(),
            None => button.into_any_element(),
        }
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Context, IntoElement, Render, TestAppContext, Window, point, px};
    use ui::IconName;

    use super::*;

    struct Host {
        clicks: Rc<Cell<u32>>,
    }

    impl Render for Host {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let clicks = Rc::clone(&self.clicks);
            IconButton::new("button", IconName::Plus)
                .on_click(move |_, _, _| clicks.set(clicks.get() + 1))
        }
    }

    #[gpui::test]
    fn a_click_runs_the_handler(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let clicks = Rc::new(Cell::new(0));
        let shared = Rc::clone(&clicks);
        let (_, cx) = cx.add_window_view(|_, _| Host { clicks: shared });
        cx.simulate_click(point(px(4.0), px(4.0)), gpui::Modifiers::none());
        assert_eq!(clicks.get(), 1);
    }

    #[test]
    fn the_tile_shape_rounds_more_than_the_small_one() {
        let small = IconButton::new("a", IconName::Plus);
        let tile = IconButton::new("b", IconName::Plus).shape(TileShape::Tile);
        assert!(tile.radius() > small.radius());
    }
}
