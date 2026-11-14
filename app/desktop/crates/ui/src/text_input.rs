//! A single-line text field with selection, clipboard and input-method support.

use gpui::AppContext as _;
use gpui::{
    App, Bounds, Context, CursorStyle, Entity, EventEmitter, FocusHandle, Focusable,
    InteractiveElement, IntoElement, MouseButton, ParentElement, Pixels, Render, ShapedLine,
    SharedString, Styled, Window, div, px,
};

use crate::chrome::chrome;
use crate::font::UI_FAMILY;
use crate::opaque_debug::opaque_debug;
use crate::radius;
use crate::surface::Surface;
use crate::text_buffer::TextBuffer;
use crate::text_input_element::TextInputElement;
use crate::text_input_event::TextInputEvent;
use crate::text_size;
use crate::theme::Theme;

const CONTROL_HEIGHT: f32 = 32.0;
const PADDING: f32 = 10.0;

/// A text field entity; render it as a child and subscribe to [`TextInputEvent`].
pub struct TextInput {
    pub(crate) buffer: TextBuffer,
    placeholder: SharedString,
    pub(crate) focus_handle: FocusHandle,
    pub(crate) layout: Option<ShapedLine>,
    pub(crate) bounds: Option<Bounds<Pixels>>,
    pub(crate) selecting: bool,
    surface: Surface,
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
        let field = div()
            .id("ui-text-input")
            .key_context(crate::text_input_actions::CONTEXT)
            .track_focus(&self.focus_handle)
            .on_mouse_down(MouseButton::Left, cx.listener(Self::on_mouse_down))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_move(cx.listener(Self::on_mouse_move))
            .cursor(CursorStyle::IBeam);
        Self::bind_actions(field, cx)
            .flex()
            .items_center()
            .w_full()
            .h(px(CONTROL_HEIGHT))
            .px(px(PADDING))
            .rounded(radius::LG)
            .border_1()
            .border_color(frame.border)
            .bg(frame.fill)
            .text_color(palette.foreground())
            .text_size(text_size::SM)
            .line_height(px(20.0))
            .font_family(UI_FAMILY)
            .shadow(frame.shadows)
            .child(TextInputElement::new(cx.entity(), palette))
    }
}

opaque_debug!(TextInput);

#[cfg(test)]
#[path = "text_input_tests.rs"]
mod tests;
