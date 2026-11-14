//! Header strip of a dialog: icon, title, extra controls and the close button.

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, App, ElementId, IntoElement, ParentElement, RenderOnce, SharedString, Styled,
    Window, div, px,
};

use crate::button::Button;
use crate::button_size::ButtonSize;
use crate::button_variant::ButtonVariant;
use crate::close_handler::CloseHandler;
use crate::font::HEADING_FAMILY;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::opaque_debug::opaque_debug;
use crate::surface::Surface;
use crate::text_size;
use crate::theme::Theme;

#[derive(IntoElement)]
pub(crate) struct DialogHeader {
    id: ElementId,
    title: Option<SharedString>,
    icon: Option<IconName>,
    actions: Option<AnyElement>,
    on_close: Option<CloseHandler>,
    ruled: bool,
}

impl DialogHeader {
    pub(crate) fn new(id: ElementId) -> Self {
        Self {
            id,
            title: None,
            icon: None,
            actions: None,
            on_close: None,
            ruled: true,
        }
    }

    pub(crate) fn title(mut self, title: Option<SharedString>) -> Self {
        self.title = title;
        self
    }

    pub(crate) fn icon(mut self, icon: Option<IconName>) -> Self {
        self.icon = icon;
        self
    }

    pub(crate) fn actions(mut self, actions: Option<AnyElement>) -> Self {
        self.actions = actions;
        self
    }

    pub(crate) fn on_close(mut self, handler: Option<CloseHandler>) -> Self {
        self.on_close = handler;
        self
    }

    pub(crate) fn unruled(mut self) -> Self {
        self.ruled = false;
        self
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.title.is_none() && self.icon.is_none() && self.actions.is_none()
    }
}

impl RenderOnce for DialogHeader {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let close = Button::icon_only((self.id, "close"), IconName::X)
            .variant(ButtonVariant::Ghost)
            .size(ButtonSize::Sm);
        let close = match self.on_close {
            Some(handler) => close.on_click(move |_, window, cx| handler(window, cx)),
            None => close,
        };
        div()
            .flex()
            .flex_none()
            .items_center()
            .gap(px(8.0))
            .px(px(16.0))
            .py(px(12.0))
            .when(self.ruled, |d| {
                d.border_b_1().border_color(palette.border())
            })
            .children(
                self.icon
                    .map(|icon| Icon::new(icon).color(palette.muted_foreground())),
            )
            .children(self.title.map(|title| {
                div()
                    .font_family(HEADING_FAMILY)
                    .text_size(text_size::SM)
                    .line_height(px(14.0))
                    .child(title)
            }))
            .when_some(self.actions, |d, actions| d.child(actions_slot(actions)))
            .child(div().ml_auto().child(close))
    }
}

fn actions_slot(actions: AnyElement) -> impl IntoElement {
    div()
        .ml(px(8.0))
        .flex()
        .items_center()
        .gap(px(8.0))
        .child(actions)
}

opaque_debug!(DialogHeader);

#[cfg(test)]
#[path = "dialog_header_tests.rs"]
mod tests;
