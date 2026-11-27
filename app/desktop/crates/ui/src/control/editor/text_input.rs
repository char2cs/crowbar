//! A single-line text field with selection, clipboard and input-method support.

pub(crate) mod binding;
mod commands;
mod fields;
mod input_method;
mod line_element;
mod mouse;
mod render;
mod select_all;

pub(crate) use binding::actions::{
    Backspace, Cancel, Copy, Cut, Delete, End, Home, Left, Paste, Right, SelectAll, SelectLeft,
    SelectRight, Submit, bindings, word_bindings,
};
pub(crate) use line_element::PLACEHOLDER_OPACITY;
pub(crate) use line_element::caret_scroll::CARET_WIDTH;

use gpui::AppContext as _;
use gpui::{
    App, Bounds, Context, Entity, EventEmitter, FocusHandle, Focusable, Pixels, SharedString,
    Window,
};

use crate::control::editor::buffer::Buffer;
use crate::control::editor::field::Blink;
use crate::control::editor::text_input_event::TextInputEvent;
use crate::control::editor::text_input_style::TextInputStyle;
use crate::display::TrackedLine;
use crate::foundation::IconName;
use crate::foundation::Surface;
use crate::foundation::opaque_debug;

/// A text field entity; render it as a child and subscribe to [`TextInputEvent`].
pub struct TextInput {
    pub(crate) buffer: Buffer,
    pub(crate) placeholder: SharedString,
    pub(crate) focus_handle: FocusHandle,
    pub(crate) layout: Option<TrackedLine>,
    pub(crate) bounds: Option<Bounds<Pixels>>,
    pub(crate) scroll: Pixels,
    pub(crate) selecting: bool,
    pub(crate) surface: Surface,
    pub(crate) style: TextInputStyle,
    pub(crate) leading: Option<IconName>,
    pub(crate) blink: Blink,
}

impl EventEmitter<TextInputEvent> for TextInput {}

impl Focusable for TextInput {
    fn focus_handle(
        &self,
        _cx: &App,
    ) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl TextInput {
    /// An empty field showing `placeholder` while empty.
    pub fn new(
        placeholder: impl Into<SharedString>,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        Self {
            buffer: Buffer::default(),
            placeholder: placeholder.into(),
            focus_handle: cx.focus_handle().tab_stop(true),
            layout: None,
            bounds: None,
            scroll: Pixels::ZERO,
            selecting: false,
            surface: Surface::default(),
            style: TextInputStyle::default(),
            leading: None,
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

    /// Sets the size and finish of the field.
    #[must_use]
    pub fn style(
        mut self,
        style: TextInputStyle,
    ) -> Self {
        self.style = style;
        self
    }

    /// Shows `icon` inside the left edge and moves the text clear of it.
    #[must_use]
    pub fn leading_icon(
        mut self,
        icon: IconName,
    ) -> Self {
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
    pub fn set_text(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.buffer.set_text(text);
        self.restart_blink(cx);
    }

    /// Moves keyboard focus to the field.
    pub fn focus(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) {
        window.focus(&self.focus_handle, cx);
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

opaque_debug!(TextInput);

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{App, Entity, TestAppContext, VisualTestContext};

    use super::TextInput;
    use crate::TextInputEvent;
    use crate::control::editor::test_support::field::{field, text_of};

    fn recorder(
        sink: Rc<RefCell<Vec<String>>>
    ) -> impl FnMut(Entity<TextInput>, &TextInputEvent, &mut App) {
        move |_, event, _| sink.borrow_mut().extend(changed(event).cloned())
    }

    fn changed(event: &TextInputEvent) -> Option<&String> {
        match event {
            TextInputEvent::Changed(text) => Some(text),
            _ => None,
        }
    }

    fn record_changes(
        input: &Entity<TextInput>,
        cx: &mut VisualTestContext,
    ) -> Rc<RefCell<Vec<String>>> {
        let seen = Rc::new(RefCell::new(Vec::new()));
        let sink = Rc::clone(&seen);
        cx.update(|_, cx| {
            cx.subscribe(input, recorder(sink)).detach();
        });
        seen
    }

    #[gpui::test]
    fn typing_inserts_at_the_caret(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.simulate_input("héllo");
        assert_eq!(text_of(&input, cx), "héllo");
    }

    #[gpui::test]
    fn set_text_replaces_and_the_placeholder_is_kept(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        input.update(cx, |input, cx| input.set_text("query", cx));
        assert_eq!(text_of(&input, cx), "query");
        assert_eq!(
            input.read_with(cx, |input, _| input.placeholder().to_string()),
            "Search"
        );
    }

    #[gpui::test]
    fn editing_notifies_subscribers_with_the_new_text(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        let seen = record_changes(&input, cx);
        cx.simulate_input("ab");
        assert_eq!(seen.borrow().last().map(String::as_str), Some("ab"));
    }

    #[gpui::test]
    fn tab_reaches_the_field_as_it_reaches_an_input_in_the_browser(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        cx.update(gpui::Window::blur);
        cx.run_until_parked();
        let focused = cx.update(|window, cx| {
            window.focus_next(cx);
            input.read(cx).focus_handle.is_focused(window)
        });
        assert!(focused);
    }
}
