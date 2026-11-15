//! Text and icon buttons.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ClickEvent, Div, ElementId, FontWeight, Hsla, InteractiveElement, IntoElement,
    MouseButton, ParentElement, RenderOnce, SharedString, StatefulInteractiveElement,
    StyleRefinement, Styled, Window, div, hsla,
};

use crate::button_colors::ButtonColors;
use crate::button_size::ButtonSize;
use crate::button_variant::ButtonVariant;
use crate::click_handler::ClickHandler;
use crate::focus_ring::focus_ring;
use crate::hover;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::opaque_debug::opaque_debug;
use crate::pose::Pose;
use crate::spinner::Spinner;
use crate::surface::Surface;
use crate::text_size;
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
    loading: bool,
    on_click: Option<ClickHandler>,
}

impl Button {
    /// A labelled button.
    pub fn new(
        id: impl Into<ElementId>,
        label: impl Into<SharedString>,
    ) -> Self {
        Self::bare(id.into(), Some(label.into()), None)
    }

    /// A square button showing only `icon`; defaults to the ghost variant.
    pub fn icon_only(
        id: impl Into<ElementId>,
        icon: IconName,
    ) -> Self {
        Self::bare(id.into(), None, Some(icon)).variant(ButtonVariant::Ghost)
    }

    fn bare(
        id: ElementId,
        label: Option<SharedString>,
        icon: Option<IconName>,
    ) -> Self {
        Self {
            id,
            label,
            icon,
            variant: ButtonVariant::default(),
            size: ButtonSize::default(),
            surface: Surface::default(),
            disabled: false,
            loading: false,
            on_click: None,
        }
    }

    /// Adds a leading icon to a labelled button.
    #[must_use]
    pub fn icon(
        mut self,
        icon: IconName,
    ) -> Self {
        self.icon = Some(icon);
        self
    }

    /// Chooses the colour variant.
    #[must_use]
    pub fn variant(
        mut self,
        variant: ButtonVariant,
    ) -> Self {
        self.variant = variant;
        self
    }

    /// Chooses the size step.
    #[must_use]
    pub fn size(
        mut self,
        size: ButtonSize,
    ) -> Self {
        self.size = size;
        self
    }

    /// Paints the button with the palette of `surface`.
    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }

    /// Disables the button: dimmed, no hover, no click.
    #[must_use]
    pub fn disabled(mut self) -> Self {
        self.disabled = true;
        self
    }

    /// Hides the label behind a centred spinner and ignores clicks, as the React `loading` prop does.
    #[must_use]
    pub fn loading(mut self) -> Self {
        self.loading = true;
        self
    }

    /// Registers the activation handler.
    #[must_use]
    pub fn on_click(
        mut self,
        handler: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_click = Some(Box::new(handler));
        self
    }
}

impl Button {
    fn frame(
        &self,
        colors: &ButtonColors,
        background: Hsla,
    ) -> Div {
        let size = self.size;
        let frame = div()
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .gap(size.gap())
            .h(size.height())
            .rounded(size.radius())
            .border_1()
            .border_color(colors.border)
            .bg(background)
            .text_color(colors.foreground)
            .text_size(size.text())
            .font_weight(FontWeight::MEDIUM)
            .line_height(text_size::LINE_HEIGHT)
            .whitespace_nowrap()
            .shadow(colors.shadow.clone());
        match &self.label {
            None => frame.w(size.height()),
            Some(_) => frame.px(size.padding_x()),
        }
    }

    fn content_color(
        &self,
        colors: &ButtonColors,
    ) -> Hsla {
        if self.loading {
            return hsla(0.0, 0.0, 0.0, 0.0);
        }
        colors.foreground
    }

    fn enabled(&self) -> bool {
        !(self.disabled || self.loading)
    }
}

impl RenderOnce for Button {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let colors = ButtonColors::resolve(self.variant, palette, theme.appearance());
        let hover = hover::track(&self.id, Pose::Off, window, cx);
        let background = hover::color(
            &hover,
            (colors.background, colors.hover_background),
            window,
            cx,
        );
        let content = self.content_color(&colors);
        let icon = self
            .icon
            .map(|name| Icon::new(name).size(self.size.icon()).color(content));
        let spinner = self
            .loading
            .then(|| indicator(&self.id, &colors, self.size));
        let (ring, enabled) = (focus_ring(palette), self.enabled());
        self.frame(&colors, background)
            .font_family(theme.ui_family())
            .relative()
            .id(self.id)
            .when(enabled, |d| {
                d.cursor_pointer()
                    .on_hover(hover::listener(&hover))
                    .tab_stop(true)
            })
            .when(!enabled, |d| d.opacity(DISABLED_OPACITY))
            .when(enabled, |d| d.active(pressed(&colors)))
            .focus(move |s| s.shadow(ring))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
            .children(icon)
            .children(self.label)
            .children(spinner)
            .when_some(self.on_click.filter(|_| enabled), |d, handler| {
                d.on_click(handler)
            })
    }
}

fn pressed(colors: &ButtonColors) -> impl FnOnce(StyleRefinement) -> StyleRefinement + use<> {
    let (fill, shadow) = (colors.pressed_background, Vec::new());
    move |style| style.bg(fill).shadow(shadow)
}

fn indicator(
    id: &ElementId,
    colors: &ButtonColors,
    size: ButtonSize,
) -> Div {
    let spinner = Spinner::new((id.clone(), "loading"))
        .size(size.icon())
        .color(colors.foreground);
    div()
        .absolute()
        .inset_0()
        .flex()
        .items_center()
        .justify_center()
        .child(spinner)
}

opaque_debug!(Button);

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Modifiers, TestAppContext, point, px};

    use super::*;
    use crate::test_support::{measure, open};

    #[gpui::test]
    fn click_invokes_the_handler(cx: &mut TestAppContext) {
        let clicks = Rc::new(Cell::new(0));
        let sink = Rc::clone(&clicks);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            Button::new("go", "Go").on_click(move |_, _, _| sink.set(sink.get() + 1))
        });
        cx.simulate_click(point(px(20.0), px(16.0)), Modifiers::none());
        assert_eq!(clicks.get(), 1);
    }

    #[gpui::test]
    fn disabled_button_ignores_clicks(cx: &mut TestAppContext) {
        let clicks = Rc::new(Cell::new(0));
        let sink = Rc::clone(&clicks);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            Button::new("go", "Go")
                .disabled()
                .on_click(move |_, _, _| sink.set(sink.get() + 1))
        });
        cx.simulate_click(point(px(20.0), px(16.0)), Modifiers::none());
        assert_eq!(clicks.get(), 0);
    }

    fn square(size: ButtonSize) -> Button {
        Button::icon_only("b", IconName::Gear).size(size)
    }

    #[gpui::test]
    fn sizes_set_the_height_and_icon_buttons_are_square(cx: &mut TestAppContext) {
        for size in [
            ButtonSize::Xs,
            ButtonSize::Sm,
            ButtonSize::Md,
            ButtonSize::Lg,
            ButtonSize::Xl,
        ] {
            let labelled = measure(cx, move |_, _| Button::new("a", "Go").size(size));
            let icon = measure(cx, move |_, _| square(size));
            assert_eq!(labelled.size.height, size.height(), "{size:?}");
            assert_eq!(icon.size.width, icon.size.height, "{size:?}");
        }
    }

    #[gpui::test]
    fn loading_button_ignores_clicks(cx: &mut TestAppContext) {
        let clicks = Rc::new(Cell::new(0));
        let sink = Rc::clone(&clicks);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            Button::new("go", "Go")
                .loading()
                .on_click(move |_, _, _| sink.set(sink.get() + 1))
        });
        cx.simulate_click(point(px(20.0), px(16.0)), Modifiers::none());
        assert_eq!(clicks.get(), 0);
    }

    #[gpui::test]
    fn loading_keeps_the_button_size(cx: &mut TestAppContext) {
        let plain = measure(cx, |_, _| Button::new("go", "Go"));
        let loading = measure(cx, |_, _| Button::new("go", "Go").loading());
        assert_eq!(plain.size, loading.size);
    }
}
