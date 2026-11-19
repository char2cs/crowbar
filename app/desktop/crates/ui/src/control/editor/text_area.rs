//! A multi-line text field with wrapping, selection, undo, clipboard and input-method support.

mod binding;
mod editing;
mod fields;
mod input_method;
mod layout;
mod mouse;
mod render;
mod wrapped_element;

pub(crate) use binding::bindings;
pub use render::{TextAreaCondition, TextAreaSize};

use gpui::AppContext as _;
use gpui::{
    App, Bounds, Context, Entity, EventEmitter, FocusHandle, Focusable, Pixels, Point,
    SharedString, Task, Window, px,
};

use crate::control::editor::buffer::Buffer;
use crate::control::editor::field::Blink;
use crate::control::editor::history::History;
use crate::control::editor::text_area::layout::Layout;
use crate::control::editor::text_input_event::TextInputEvent;
use crate::foundation::Surface;
use crate::foundation::opaque_debug;

/// A multi-line text field entity; render it as a child and subscribe to [`TextInputEvent`].
///
/// `Enter` inserts a newline, `Ctrl+Enter` (`Cmd+Enter` on macOS) emits `Submitted`, `Escape` emits
/// `Cancelled`. The field grows with its text up to [`TextArea::max_height`], then scrolls.
pub struct TextArea {
    pub(crate) buffer: Buffer,
    pub(crate) history: History,
    pub(crate) placeholder: SharedString,
    pub(crate) focus_handle: FocusHandle,
    pub(crate) layout: Layout,
    pub(crate) bounds: Option<Bounds<Pixels>>,
    pub(crate) scroll: Pixels,
    pub(crate) revealed: usize,
    pub(crate) goal: Option<(usize, Pixels)>,
    pub(crate) selecting: bool,
    pub(crate) pointer: Point<Pixels>,
    pub(crate) autoscroll: Option<Task<()>>,
    pub(crate) surface: Surface,
    pub(crate) size: TextAreaSize,
    pub(crate) limit: Option<Pixels>,
    pub(crate) condition: TextAreaCondition,
    pub(crate) blink: Blink,
}

impl EventEmitter<TextInputEvent> for TextArea {}

impl Focusable for TextArea {
    fn focus_handle(
        &self,
        _cx: &App,
    ) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl TextArea {
    /// An empty field showing `placeholder` while empty.
    pub fn new(
        placeholder: impl Into<SharedString>,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        Self {
            buffer: Buffer::default(),
            history: History::default(),
            placeholder: placeholder.into(),
            focus_handle: cx.focus_handle(),
            layout: Layout::default(),
            bounds: None,
            scroll: px(0.0),
            revealed: 0,
            goal: None,
            selecting: false,
            pointer: Point::default(),
            autoscroll: None,
            surface: Surface::default(),
            size: TextAreaSize::default(),
            limit: None,
            condition: TextAreaCondition::default(),
            blink: Blink::default(),
        }
    }

    /// Builds a field entity in one call.
    pub fn build(
        placeholder: impl Into<SharedString>,
        cx: &mut App,
    ) -> Entity<Self> {
        cx.new(|cx| Self::new(placeholder, cx))
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

    /// The current text.
    pub fn text(&self) -> &str {
        self.buffer.text()
    }

    /// The placeholder shown while the text is empty.
    pub fn placeholder(&self) -> &SharedString {
        &self.placeholder
    }

    /// Replaces the text and selects its end, without emitting `Changed`; the undo history starts over.
    pub fn set_text(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.history = History::default();
        self.buffer.set_text(text);
        self.restart_blink(cx);
    }

    /// Moves keyboard focus to the field.
    pub fn focus(
        &self,
        window: &mut Window,
    ) {
        window.focus(&self.focus_handle);
    }

    pub(crate) fn accepts_input(&self) -> bool {
        self.condition.accepts_input()
    }

    pub(crate) fn changed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        cx.emit(TextInputEvent::Changed(self.buffer.text().to_owned()));
        self.restart_blink(cx);
    }

    pub(crate) fn restart_blink(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.blink.restart();
        cx.notify();
    }
}

opaque_debug!(TextArea);

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::control::editor::test_support::area::{area, area_text};

    #[gpui::test]
    fn typing_enter_and_text_build_lines(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        cx.simulate_input("one");
        cx.simulate_keystrokes("enter");
        cx.simulate_input("two");
        assert_eq!(area_text(&area, cx), "one\ntwo");
    }

    #[gpui::test]
    fn set_text_replaces_and_keeps_the_placeholder(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        area.update(cx, |area, cx| area.set_text("a\nb", cx));
        assert_eq!(area_text(&area, cx), "a\nb");
        let hint = area.read_with(cx, |area, _| area.placeholder().to_string());
        assert_eq!(hint, "Write something");
    }
}
