//! The grip of a draggable surface.

use gpui::StatefulInteractiveElement as _;
use gpui::{
    App, CursorStyle, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    Styled, Window, div, px,
};

use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::IconWeight;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::motion::hover;
use crate::foundation::opaque_debug;

const GRIP: f32 = 10.0;
const BOX: f32 = 12.0;

/// A 12px hit area with a `DotsSixVertical` grip turned on its side; muted until hovered.
#[derive(IntoElement)]
pub struct DragHandle {
    id: ElementId,
    surface: Surface,
}

impl DragHandle {
    /// A handle; attach `on_drag` to the returned element's parent or wrap it.
    pub fn new(id: impl Into<ElementId>) -> Self {
        Self {
            id: id.into(),
            surface: Surface::default(),
        }
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }
}

impl RenderOnce for DragHandle {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(self.surface);
        let state = hover::track(&self.id, Pose::Off, window, cx);
        let color = hover::color(
            &state,
            palette.muted_foreground()..palette.foreground(),
            window,
            cx,
        );
        div()
            .id(self.id)
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .size(px(BOX))
            .cursor(CursorStyle::OpenHand)
            .on_hover(hover::listener(&state))
            .child(
                Icon::new(IconName::DotsSixVertical)
                    .weight(IconWeight::Bold)
                    .size(px(GRIP))
                    .rotate(std::f32::consts::FRAC_PI_2)
                    .color(color),
            )
    }
}

opaque_debug!(DragHandle);

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::probe::measure;

    #[gpui::test]
    fn handle_is_twelve_pixels_square(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| DragHandle::new("h"));
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (12.0, 12.0)
        );
    }
}
