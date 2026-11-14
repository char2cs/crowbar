//! Text and icon buttons.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ClickEvent, ElementId, FontWeight, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::button_colors::ButtonColors;
use crate::button_size::ButtonSize;
use crate::button_variant::ButtonVariant;
use crate::click_handler::ClickHandler;
use crate::focus_ring::focus_ring;
use crate::font::UI_FAMILY;
use crate::hover;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::surface::Surface;
use crate::theme::Theme;

const DISABLED_OPACITY: f32 = 0.64;

/// A button with an optional icon and label; with only an icon it is square.
#[derive(IntoElement)]
pub struct Button {
    id: ElementId,
    label: Option<SharedString>,
    icon: Option<IconName>,
    variant: ButtonVariant,
    size: ButtonSize,
    surface: Surface,
    disabled: bool,
    on_click: Option<ClickHandler>,
}

impl Button {
    /// A labelled button.
    pub fn new(id: impl Into<ElementId>, label: impl Into<SharedString>) -> Self {
        Self::bare(id.into(), Some(label.into()), None)
    }

    /// A square button showing only `icon`; defaults to the ghost variant.
    pub fn icon_only(id: impl Into<ElementId>, icon: IconName) -> Self {
        Self::bare(id.into(), None, Some(icon)).variant(ButtonVariant::Ghost)
    }

    fn bare(id: ElementId, label: Option<SharedString>, icon: Option<IconName>) -> Self {
        Self {
            id,
            label,
            icon,
            variant: ButtonVariant::default(),
            size: ButtonSize::default(),
            surface: Surface::default(),
            disabled: false,
            on_click: None,
        }
    }

    /// Adds a leading icon to a labelled button.
    #[must_use]
    pub fn icon(mut self, icon: IconName) -> Self {
        self.icon = Some(icon);
        self
    }

    /// Chooses the colour variant.
    #[must_use]
    pub fn variant(mut self, variant: ButtonVariant) -> Self {
        self.variant = variant;
        self
    }

    /// Chooses the size step.
    #[must_use]
    pub fn size(mut self, size: ButtonSize) -> Self {
        self.size = size;
        self
    }

    /// Paints the button with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }

    /// Disables the button: dimmed, no hover, no click.
    #[must_use]
    pub fn disabled(mut self, disabled: bool) -> Self {
        self.disabled = disabled;
        self
    }

    /// Registers the activation handler.
    #[must_use]
    pub fn on_click(mut self, handler: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static) -> Self {
        self.on_click = Some(Box::new(handler));
        self
    }
}

impl RenderOnce for Button {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let colors = ButtonColors::resolve(self.variant, palette, theme.appearance());
        let size = self.size;
        let square = self.label.is_none();
        let ring = focus_ring(palette);
        let pointer = !self.disabled;
        let hover = hover::track(&self.id, "hover", false, window, cx);
        let background = hover::color(&hover, colors.background, colors.hover_background, window, cx);
        let icon = self.icon.map(|name| Icon::new(name).size(size.icon()).color(colors.foreground));
        div()
            .id(self.id)
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .gap(size.gap())
            .h(size.height())
            .map(|d| if square { d.w(size.height()) } else { d.px(size.padding_x()) })
            .rounded(size.radius())
            .border_1()
            .border_color(colors.border)
            .bg(background)
            .text_color(colors.foreground)
            .text_size(size.text())
            .font_family(UI_FAMILY)
            .font_weight(FontWeight::MEDIUM)
            .line_height(px(20.0))
            .whitespace_nowrap()
            .shadow(colors.shadow)
            .when(pointer, |d| d.cursor_pointer().on_hover(hover::listener(&hover)).tab_stop(true))
            .when(self.disabled, |d| d.opacity(DISABLED_OPACITY))
            .focus(move |s| s.shadow(ring))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
            .children(icon)
            .children(self.label)
            .when_some(self.on_click.filter(|_| pointer), |d, handler| d.on_click(handler))
    }
}

#[cfg(test)]
#[path = "button_tests.rs"]
mod tests;
