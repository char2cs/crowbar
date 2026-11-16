//! A multi-line text field with wrapping, selection, undo, clipboard and input-method support.

use std::ops::Range;

use gpui::AppContext as _;
use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, Bounds, Context, CursorStyle, Entity, EventEmitter, FocusHandle, Focusable,
    InteractiveElement, IntoElement, MouseButton, ParentElement, Pixels, Render, SharedString,
    Styled, Window, div, px,
};

use crate::opaque_debug::opaque_debug;
use crate::pose::Pose;
use crate::radius;
use crate::surface::Surface;
use crate::text_area_actions::CONTEXT;
use crate::text_area_chrome::text_area_chrome;
use crate::text_area_condition::TextAreaCondition;
use crate::text_area_element::TextAreaElement;
use crate::text_area_layout::TextAreaLayout;
use crate::text_area_size::TextAreaSize;
use crate::text_area_surface::TextAreaSurface;
use crate::text_buffer::TextBuffer;
use crate::text_history::TextHistory;
use crate::text_input_event::TextInputEvent;
use crate::text_size;
use crate::theme::Theme;

const DISABLED_OPACITY: f32 = 0.64;

/// A multi-line text field entity; render it as a child and subscribe to [`TextInputEvent`].
///
/// `Enter` inserts a newline, `Ctrl+Enter` (`Cmd+Enter` on macOS) emits `Submitted`, `Escape` emits
/// `Cancelled`. The field grows with its text up to [`TextArea::max_height`], then scrolls.
pub struct TextArea {
    pub(crate) buffer: TextBuffer,
    pub(crate) history: TextHistory,
    placeholder: SharedString,
    pub(crate) focus_handle: FocusHandle,
    pub(crate) layout: TextAreaLayout,
    pub(crate) bounds: Option<Bounds<Pixels>>,
    pub(crate) scroll: Pixels,
    pub(crate) revealed: usize,
    pub(crate) goal: Option<(usize, Pixels)>,
    pub(crate) selecting: bool,
    surface: Surface,
    size: TextAreaSize,
    limit: Option<Pixels>,
    condition: TextAreaCondition,
}

impl EventEmitter<TextInputEvent> for TextArea {}

impl TextAreaSurface for TextArea {
    fn content(&self) -> &str {
        self.buffer.text()
    }

    fn hint(&self) -> &SharedString {
        &self.placeholder
    }

    fn marked_range(&self) -> Option<Range<usize>> {
        self.buffer.marked()
    }

    fn selected_range(&self) -> Range<usize> {
        self.buffer.selection()
    }

    fn cursor(&self) -> usize {
        self.buffer.cursor()
    }

    fn scroll(&self) -> Pixels {
        self.scroll
    }

    fn revealed(&self) -> usize {
        self.revealed
    }

    fn editable(&self) -> bool {
        self.accepts_input()
    }

    fn input_focus(&self) -> FocusHandle {
        self.focus_handle.clone()
    }

    fn record_layout(
        &mut self,
        layout: TextAreaLayout,
        bounds: Bounds<Pixels>,
        scroll: Pixels,
    ) {
        self.layout = layout;
        self.bounds = Some(bounds);
        self.scroll = scroll;
        self.revealed = self.buffer.cursor();
    }
}

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
            buffer: TextBuffer::default(),
            history: TextHistory::default(),
            placeholder: placeholder.into(),
            focus_handle: cx.focus_handle(),
            layout: TextAreaLayout::default(),
            bounds: None,
            scroll: px(0.0),
            revealed: 0,
            goal: None,
            selecting: false,
            surface: Surface::default(),
            size: TextAreaSize::default(),
            limit: None,
            condition: TextAreaCondition::default(),
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

    /// Sets the minimum height and the padding.
    #[must_use]
    pub fn size(
        mut self,
        size: TextAreaSize,
    ) -> Self {
        self.size = size;
        self
    }

    /// Stops growing at `height` (border included) and scrolls beyond it.
    #[must_use]
    pub fn max_height(
        mut self,
        height: Pixels,
    ) -> Self {
        self.limit = Some(height);
        self
    }

    /// Starts disabled.
    #[must_use]
    pub fn disabled(mut self) -> Self {
        self.condition = TextAreaCondition::Disabled;
        self
    }

    /// Starts flagged invalid.
    #[must_use]
    pub fn invalid(mut self) -> Self {
        self.condition = TextAreaCondition::Invalid;
        self
    }

    /// Changes how the field presents itself.
    pub fn set_condition(
        &mut self,
        condition: TextAreaCondition,
        cx: &mut Context<'_, Self>,
    ) {
        self.condition = condition;
        self.selecting = false;
        cx.notify();
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
        self.history = TextHistory::default();
        self.buffer.set_text(text);
        cx.notify();
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
        cx.notify();
    }
}

impl Render for TextArea {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let focus = Pose::from(self.focus_handle.is_focused(window) && self.accepts_input());
        let frame = text_area_chrome(palette, theme.appearance(), focus, self.condition);
        let limit = self.limit.map(|height| height - self.size.frame_height());
        let field = div().id("ui-text-area").key_context(CONTEXT);
        let field = self.interactive(field, cx);
        Self::bind_actions(field, cx)
            .relative()
            .w_full()
            .min_h(self.size.min_height())
            .px(self.size.across())
            .py(self.size.down())
            .rounded(radius::LG)
            .border_1()
            .border_color(frame.border)
            .bg(frame.fill)
            .shadow(frame.shadows)
            .text_color(palette.foreground())
            .text_size(text_size::SM)
            .line_height(text_size::LINE_HEIGHT)
            .font_family(theme.ui_family())
            .when(!self.accepts_input(), |field| {
                field.opacity(DISABLED_OPACITY)
            })
            .child(TextAreaElement::new(cx.entity(), palette, limit))
    }
}

impl TextArea {
    fn interactive(
        &self,
        field: gpui::Stateful<gpui::Div>,
        cx: &mut Context<'_, Self>,
    ) -> gpui::Stateful<gpui::Div> {
        if !self.accepts_input() {
            return field;
        }
        field
            .track_focus(&self.focus_handle)
            .on_mouse_down(MouseButton::Left, cx.listener(Self::on_mouse_down))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_move(cx.listener(Self::on_mouse_move))
            .on_scroll_wheel(cx.listener(Self::on_scroll))
            .cursor(CursorStyle::IBeam)
    }
}

opaque_debug!(TextArea);

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::test_support::{area, area_text};

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
