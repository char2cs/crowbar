//! Centred placeholder for an empty list.

use gpui::{
    AnyElement, App, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px,
};

use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::ScaledText;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::spacing;

const MIN_HEIGHT: gpui::Pixels = px(96.0);
const MESSAGE_MAX_WIDTH: gpui::Pixels = px(180.0);
const HINT_LINE_HEIGHT: gpui::Pixels = px(15.0);

/// An icon, a message and an optional description and action, centred in 96px or more.
#[derive(IntoElement)]
pub struct EmptyState {
    message: SharedString,
    icon: Option<IconName>,
    description: Option<SharedString>,
    action: Option<AnyElement>,
    surface: Surface,
}

impl EmptyState {
    /// An empty state showing `message`.
    pub fn new(message: impl Into<SharedString>) -> Self {
        Self {
            message: message.into(),
            icon: None,
            description: None,
            action: None,
            surface: Surface::default(),
        }
    }

    /// Shows an icon above the message.
    #[must_use]
    pub fn icon(
        mut self,
        icon: IconName,
    ) -> Self {
        self.icon = Some(icon);
        self
    }

    /// Adds a smaller line below the message.
    #[must_use]
    pub fn description(
        mut self,
        description: impl Into<SharedString>,
    ) -> Self {
        self.description = Some(description.into());
        self
    }

    /// Adds an action element (a ghost button) below the description.
    #[must_use]
    pub fn action(
        mut self,
        action: impl IntoElement,
    ) -> Self {
        self.action = Some(action.into_any_element());
        self
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

impl RenderOnce for EmptyState {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let muted = theme.palette(self.surface).muted_foreground();
        div()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .gap(spacing::STEP_1_5)
            .min_h(MIN_HEIGHT)
            .px(spacing::STEP_3)
            .py(spacing::STEP_6)
            .text_color(muted)
            .font_family(theme.ui_family())
            .children(self.icon.map(|icon| {
                div()
                    .mb(px(2.0))
                    .size(spacing::STEP_7)
                    .flex()
                    .items_center()
                    .justify_center()
                    .child(Icon::new(icon).size(spacing::STEP_5).color(muted))
            }))
            .child(
                div()
                    .text_size(ScaledText::Sm.size(&theme))
                    .line_height(spacing::STEP_4)
                    .child(self.message),
            )
            .children(self.description.map(|text| {
                div()
                    .max_w(MESSAGE_MAX_WIDTH)
                    .text_size(ScaledText::Xs.size(&theme))
                    .line_height(HINT_LINE_HEIGHT)
                    .child(text)
            }))
            .children(self.action)
    }
}

opaque_debug!(EmptyState);

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::probe::measure;

    #[gpui::test]
    fn block_is_at_least_ninety_six_pixels_high(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| EmptyState::new("Nothing here"));
        assert!(f32::from(bounds.size.height) >= 96.0);
    }
}
