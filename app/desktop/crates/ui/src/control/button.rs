//! Text and icon buttons.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ClickEvent, Div, ElementId, FocusHandle, FontWeight, Hsla, InteractiveElement,
    IntoElement, MouseButton, ParentElement, Pixels, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, div, hsla,
};

use crate::control::button::click_handler::ClickHandler;
use crate::control::button::colors::Colors;
use crate::control::button::fill::Fill;
use crate::display::Spinner;
use crate::foundation::ControlScale;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::focus_ring;
use crate::foundation::motion::hover;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::text_size;

const DISABLED_OPACITY: f32 = 0.64;

/// A button with an optional icon and label; with only an icon it is square.
#[derive(IntoElement)]
pub struct Button {
    id: ElementId,
    label: Option<SharedString>,
    icon: Option<IconName>,
    icon_size: Option<Pixels>,
    variant: ButtonVariant,
    size: ButtonSize,
    surface: Surface,
    disabled: bool,
    loading: bool,
    fill: Fill,
    focus: Option<FocusHandle>,
    on_click: Option<ClickHandler>,
}

mod appearance;
mod button_size;
mod button_variant;
mod click_handler;
mod colors;
mod fill;

pub use button_size::ButtonSize;
pub use button_variant::ButtonVariant;

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
            icon_size: None,
            variant: ButtonVariant::default(),
            size: ButtonSize::default(),
            surface: Surface::default(),
            disabled: false,
            loading: false,
            fill: Fill::Content,
            focus: None,
            on_click: None,
        }
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

    /// Tracks `focus`, so its owner can focus the button; a tab stop unless `focus` is not one.
    #[must_use]
    pub fn focus_handle(
        mut self,
        focus: FocusHandle,
    ) -> Self {
        self.focus = Some(focus);
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
        colors: &Colors,
        background: Hsla,
        (height, text): (Pixels, Pixels),
    ) -> Div {
        let size = self.size;
        let frame = div()
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .gap(size.gap())
            .h(height)
            .rounded(size.radius())
            .border_1()
            .border_color(colors.border)
            .bg(background)
            .text_color(colors.foreground)
            .text_size(text)
            .font_weight(FontWeight::MEDIUM)
            .line_height(text_size::LINE_HEIGHT)
            .whitespace_nowrap()
            .shadow(colors.shadow.clone());
        let frame = frame.when(self.variant == ButtonVariant::Row, |row| {
            row.justify_start()
        });
        let frame = self.fill.apply(frame);
        match &self.label {
            None => frame.w(height),
            Some(_) => frame.px(size.padding_x()),
        }
    }

    fn content_color(
        &self,
        colors: &Colors,
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
        let colors = Colors::resolve(self.variant, palette, theme.appearance());
        let (ring, enabled) = (focus_ring(palette), self.enabled());
        let hover = hover::track(&self.id, Pose::Off, window, cx);
        let colors = colors.inked(&hover, (window, &*cx));
        let background = colors.fill(&hover, (window, cx));
        let content = self.content_color(&colors);
        let (height, text, icon_size) = self.size.sized(ControlScale::of(window));
        let icon_size = self.icon_size.unwrap_or(icon_size);
        let icon = (self.icon.map(Icon::new)).map(|icon| icon.size(icon_size).color(content));
        let spinner = self
            .loading
            .then(|| indicator(&self.id, &colors, self.size));
        self.frame(&colors, background, (height, text))
            .font_family(theme.ui_family())
            .relative()
            .id(self.id)
            .on_hover(hover::listener(&hover))
            .when(enabled, |d| {
                d.cursor_pointer()
                    .focusable()
                    .tab_stop(true)
                    .active(colors.pressed())
                    .when_some(self.focus, |d, focus| d.track_focus(&focus))
            })
            .when(!enabled, |d| {
                d.opacity(DISABLED_OPACITY).bg(colors.background)
            })
            .focus_visible(move |s| s.shadow(ring))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
            .children(icon)
            .children(self.label)
            .children(spinner)
            .when_some(self.on_click.filter(|_| enabled), |d, handler| {
                d.on_click(handler)
            })
    }
}

fn indicator(
    id: &ElementId,
    colors: &Colors,
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

    use gpui::{Modifiers, TestAppContext, VisualTestContext, point, px};

    use super::*;
    use crate::test_support::probe::{measure, open};

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

    fn rings_drawn(cx: &mut VisualTestContext) -> usize {
        cx.run_until_parked();
        cx.update(|window, cx| {
            let ring = Theme::of(cx).palette(Surface::default()).ring();
            let shadows = window.painted_shadows();
            shadows.iter().filter(|s| s.color == ring).count()
        })
    }

    #[gpui::test]
    fn the_focus_ring_shows_only_after_a_key_as_develop_s_focus_visible_does(
        cx: &mut TestAppContext
    ) {
        let focus = cx.update(|cx| cx.focus_handle());
        let held = focus.clone();
        let cx = open(cx, move |_, _| {
            Button::new("go", "Go").focus_handle(held.clone())
        });
        cx.simulate_click(point(px(20.0), px(16.0)), Modifiers::none());
        cx.update(|window, cx| window.focus(&focus, cx));
        assert_eq!(rings_drawn(cx), 0, "focused from a pointer click");
        cx.simulate_keystrokes("shift");
        assert_eq!(rings_drawn(cx), 1, "focused after a key");
    }
}
