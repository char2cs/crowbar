//! Centred placeholder for an empty list.

use gpui::{AnyElement, App, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px};

use crate::font::UI_FAMILY;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::surface::Surface;
use crate::text_size::UiText;
use crate::theme::Theme;

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
        Self { message: message.into(), icon: None, description: None, action: None, surface: Surface::default() }
    }

    /// Shows an icon above the message.
    #[must_use]
    pub fn icon(mut self, icon: IconName) -> Self {
        self.icon = Some(icon);
        self
    }

    /// Adds a smaller line below the message.
    #[must_use]
    pub fn description(mut self, description: impl Into<SharedString>) -> Self {
        self.description = Some(description.into());
        self
    }

    /// Adds an action element (a ghost button) below the description.
    #[must_use]
    pub fn action(mut self, action: impl IntoElement) -> Self {
        self.action = Some(action.into_any_element());
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }
}

impl RenderOnce for EmptyState {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let muted = theme.palette(self.surface).muted_foreground();
        div()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .gap(px(6.0))
            .min_h(px(96.0))
            .px(px(12.0))
            .py(px(24.0))
            .text_color(muted)
            .font_family(UI_FAMILY)
            .children(self.icon.map(|icon| {
                div().mb(px(2.0)).size(px(28.0)).flex().items_center().justify_center().child(Icon::new(icon).size(px(20.0)).color(muted))
            }))
            .child(div().text_size(UiText::Sm.size(&theme)).line_height(px(16.0)).child(self.message))
            .children(self.description.map(|text| {
                div().max_w(px(180.0)).text_size(UiText::Xs.size(&theme)).line_height(px(15.0)).child(text)
            }))
            .children(self.action)
    }
}

#[cfg(test)]
#[path = "empty_state_tests.rs"]
mod tests;
