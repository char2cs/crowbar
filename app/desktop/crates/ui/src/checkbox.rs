//! The checkbox.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce, StatefulInteractiveElement,
    Styled, Window, div, px,
};

use crate::appearance::Appearance;
use crate::check_handler::ToggleHandler;
use crate::focus_ring::focus_ring;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::icon_weight::IconWeight;
use crate::shadow;
use crate::surface::Surface;
use crate::theme::Theme;

const SIZE: f32 = 16.0;
const MARK: f32 = 12.0;
const RADIUS: f32 = 4.0;
const DISABLED_OPACITY: f32 = 0.64;

/// A 16px checkbox; the owner stores the value and passes it back.
#[derive(IntoElement)]
pub struct Checkbox {
    id: ElementId,
    checked: bool,
    disabled: bool,
    surface: Surface,
    on_change: Option<ToggleHandler>,
}

impl Checkbox {
    /// A checkbox showing `checked`.
    pub fn new(id: impl Into<ElementId>, checked: bool) -> Self {
        Self { id: id.into(), checked, disabled: false, surface: Surface::default(), on_change: None }
    }

    /// Disables the checkbox.
    #[must_use]
    pub fn disabled(mut self, disabled: bool) -> Self {
        self.disabled = disabled;
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }

    /// Registers the handler receiving the requested value.
    #[must_use]
    pub fn on_change(mut self, handler: impl Fn(bool, &mut Window, &mut App) + 'static) -> Self {
        self.on_change = Some(Box::new(handler));
        self
    }
}

impl RenderOnce for Checkbox {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let dark = theme.appearance() == Appearance::Dark;
        let (checked, enabled) = (self.checked, !self.disabled);
        let rest = if dark { palette.input().opacity(0.32) } else { palette.background() };
        div()
            .id(self.id)
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .size(px(SIZE))
            .rounded(px(RADIUS))
            .border_1()
            .border_color(if checked { palette.primary() } else { palette.input() })
            .bg(if checked { palette.primary() } else { rest })
            .when(!checked && enabled, |d| d.shadow(shadow::xs(0.05)))
            .when(enabled, |d| d.cursor_pointer().tab_stop(true))
            .when(!enabled, |d| d.opacity(DISABLED_OPACITY))
            .focus(move |s| s.shadow(focus_ring(palette)))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
            .when(checked, |d| {
                d.child(
                    Icon::new(IconName::Check)
                        .weight(IconWeight::Bold)
                        .size(px(MARK))
                        .color(palette.primary_foreground()),
                )
            })
            .when_some(self.on_change.filter(|_| enabled), move |d, handler| {
                d.on_click(move |_, window, cx| handler(!checked, window, cx))
            })
    }
}

#[cfg(test)]
#[path = "checkbox_tests.rs"]
mod tests;
