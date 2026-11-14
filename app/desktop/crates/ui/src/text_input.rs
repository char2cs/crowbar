//! A single-line text field with selection, clipboard and input-method support.

use gpui::AppContext as _;
use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, Bounds, Context, CursorStyle, Entity, EventEmitter, FocusHandle, Focusable,
    InteractiveElement, IntoElement, MouseButton, ParentElement, Pixels, Render, ShapedLine,
    SharedString, Styled, Window, div, px,
};

use crate::chrome::chrome;
use crate::font::UI_FAMILY;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::opaque_debug::opaque_debug;
use crate::surface::Surface;
use crate::text_buffer::TextBuffer;
use crate::text_input_element::TextInputElement;
use crate::text_input_event::TextInputEvent;
use crate::text_input_style::TextInputStyle;
use crate::theme::Theme;

const ICON_SIZE: f32 = 14.0;
const ICON_INSET: f32 = 8.0;
const ICON_CLEARANCE: f32 = 28.0;

/// A text field entity; render it as a child and subscribe to [`TextInputEvent`].
pub struct TextInput {
    pub(crate) buffer: TextBuffer,
    placeholder: SharedString,
    pub(crate) focus_handle: FocusHandle,
    pub(crate) layout: Option<ShapedLine>,
    pub(crate) bounds: Option<Bounds<Pixels>>,
    pub(crate) selecting: bool,
    surface: Surface,
    pub(crate) style: TextInputStyle,
    leading: Option<IconName>,
}

impl EventEmitter<TextInputEvent> for TextInput {}

impl Focusable for TextInput {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl TextInput {
    /// An empty field showing `placeholder` while empty.
    pub fn new(placeholder: impl Into<SharedString>, cx: &mut Context<'_, Self>) -> Self {
        Self {
            buffer: TextBuffer::default(),
            placeholder: placeholder.into(),
            focus_handle: cx.focus_handle(),
            layout: None,
            bounds: None,
            selecting: false,
            surface: Surface::default(),
            style: TextInputStyle::default(),
            leading: None,
        }
    }

    /// Builds a field entity in one call.
    pub fn build(placeholder: impl Into<SharedString>, cx: &mut App) -> Entity<Self> {
        cx.new(|cx| Self::new(placeholder, cx))
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }

    /// Sets the size and finish of the field.
    #[must_use]
    pub fn style(mut self, style: TextInputStyle) -> Self {
        self.style = style;
        self
    }

    /// Shows `icon` inside the left edge and moves the text clear of it.
    #[must_use]
    pub fn leading_icon(mut self, icon: IconName) -> Self {
        self.leading = Some(icon);
        self
    }

    /// The current text.
    pub fn text(&self) -> &str {
        self.buffer.text()
    }

    /// The placeholder shown while the text is empty.
    pub fn placeholder(&self) -> &SharedString {
        &self.placeholder
    }

    /// Replaces the text and selects its end, without emitting `Changed`.
    pub fn set_text(&mut self, text: &str, cx: &mut Context<'_, Self>) {
        self.buffer.set_text(text);
        cx.notify();
    }

    /// Moves keyboard focus to the field.
    pub fn focus(&self, window: &mut Window) {
        window.focus(&self.focus_handle);
    }

    pub(crate) fn changed(&mut self, cx: &mut Context<'_, Self>) {
        cx.emit(TextInputEvent::Changed(self.buffer.text().to_owned()));
        cx.notify();
    }
}

impl Render for TextInput {
    fn render(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let frame = chrome(
            palette,
            theme.appearance(),
            self.focus_handle.is_focused(window).into(),
        );
        let (fill, border, shadows) = self.finish(palette, frame, window);
        let field = self.field(cx);
        let left = self
            .leading
            .map_or(self.style.padding(), |_| px(ICON_CLEARANCE));
        Self::bind_actions(field, cx)
            .relative()
            .flex()
            .items_center()
            .w_full()
            .h(self.style.height())
            .pl(left)
            .pr(self.style.padding())
            .rounded(self.style.radius())
            .border_1()
            .border_color(border)
            .bg(fill)
            .text_color(palette.foreground())
            .text_size(self.style.text(theme))
            .line_height(px(20.0))
            .font_family(UI_FAMILY)
            .when(self.style == TextInputStyle::Bare, |d| {
                d.font_weight(gpui::FontWeight::MEDIUM)
            })
            .shadow(shadows)
            .children(self.leading.map(|icon| leading(icon, palette)))
            .child(TextInputElement::new(cx.entity(), palette))
    }
}

impl TextInput {
    fn field(&self, cx: &mut Context<'_, Self>) -> gpui::Stateful<gpui::Div> {
        div()
            .id("ui-text-input")
            .key_context(crate::text_input_actions::CONTEXT)
            .track_focus(&self.focus_handle)
            .on_mouse_down(MouseButton::Left, cx.listener(Self::on_mouse_down))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_move(cx.listener(Self::on_mouse_move))
            .cursor(CursorStyle::IBeam)
    }

    fn finish(
        &self,
        palette: &crate::palette::Palette,
        frame: crate::chrome::Chrome,
        window: &Window,
    ) -> (gpui::Hsla, gpui::Hsla, Vec<gpui::BoxShadow>) {
        if self.style == TextInputStyle::Bare {
            let clear = gpui::hsla(0.0, 0.0, 0.0, 0.0);
            return (clear, clear, Vec::new());
        }
        if self.style != TextInputStyle::Stepper {
            return (frame.fill, frame.border, frame.shadows);
        }
        let border = if self.focus_handle.is_focused(window) {
            frame.border
        } else {
            palette.border()
        };
        (palette.muted(), border, Vec::new())
    }
}

fn leading(icon: IconName, palette: &crate::palette::Palette) -> impl IntoElement {
    div().absolute().left(px(ICON_INSET)).child(
        Icon::new(icon)
            .size(px(ICON_SIZE))
            .color(palette.muted_foreground()),
    )
}

opaque_debug!(TextInput);

#[cfg(test)]
#[path = "text_input_tests.rs"]
mod tests;
