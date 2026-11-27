mod binding;
mod editing;
mod enter_key;
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
use crate::control::editor::text_area::enter_key::EnterKey;
use crate::control::editor::text_area::layout::Layout;
use crate::control::editor::text_input_event::TextInputEvent;
use crate::foundation::Surface;
use crate::foundation::opaque_debug;

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
    pub(crate) enter: EnterKey,
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
            enter: EnterKey::default(),
        }
    }

    pub fn build(
        placeholder: impl Into<SharedString>,
        cx: &mut App,
    ) -> Entity<Self> {
        cx.new(|cx| Self::new(placeholder, cx))
    }

    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }

    #[must_use]
    pub fn submit_on_enter(mut self) -> Self {
        self.enter = EnterKey::Submit;
        self
    }

    pub fn text(&self) -> &str {
        self.buffer.text()
    }

    pub fn placeholder(&self) -> &SharedString {
        &self.placeholder
    }

    pub fn set_text(
        &mut self,
        text: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.history = History::default();
        self.buffer.set_text(text);
        self.restart_blink(cx);
    }

    pub fn focus(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) {
        window.focus(&self.focus_handle, cx);
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
