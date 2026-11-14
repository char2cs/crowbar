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
    pub(crate) fn new(id: impl Into<ElementId>, icon: IconName) -> Self {
        Self {
            id: id.into(),
            icon,
            tooltip: None,
            shape: TileShape::Small,
            on_click: None,
        }
    }

    pub(crate) fn tooltip(mut self, text: impl Into<SharedString>) -> Self {
        self.tooltip = Some(text.into());
        self
    }

    pub(crate) fn shape(mut self, shape: TileShape) -> Self {
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
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
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
#[path = "icon_button_tests.rs"]
mod tests;
