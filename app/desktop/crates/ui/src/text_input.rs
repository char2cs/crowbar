//! A single-line text field with selection, clipboard and input-method support.

use std::ops::Range;

use gpui::{
    App, Bounds, ClipboardItem, Context, CursorStyle, Entity, EntityInputHandler, EventEmitter, FocusHandle, Focusable,
    InteractiveElement, IntoElement, MouseButton, MouseDownEvent, MouseMoveEvent, MouseUpEvent, ParentElement, Pixels,
    Point, Render, ShapedLine, SharedString, Styled, UTF16Selection, Window, div, px,
};

use crate::appearance::Appearance;
use crate::font::UI_FAMILY;
use crate::palette::Palette;
use crate::radius;
use crate::shadow;
use crate::surface::Surface;
use crate::text_buffer::TextBuffer;
use crate::text_input_actions as actions;
use crate::text_input_element::TextInputElement;
use crate::text_input_event::TextInputEvent;
use crate::text_size;
use crate::theme::Theme;

const CONTROL_HEIGHT: f32 = 32.0;
const PADDING: f32 = 10.0;
const RING_WIDTH: f32 = 3.0;

/// A text field entity; render it as a child and subscribe to [`TextInputEvent`].
pub struct TextInput {
    pub(crate) buffer: TextBuffer,
    placeholder: SharedString,
    pub(crate) focus_handle: FocusHandle,
    pub(crate) layout: Option<ShapedLine>,
    pub(crate) bounds: Option<Bounds<Pixels>>,
    selecting: bool,
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
    pub fn new(placeholder: impl Into<SharedString>, cx: &mut Context<Self>) -> Self {
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
    pub fn set_text(&mut self, text: &str, cx: &mut Context<Self>) {
        self.buffer.set_text(text);
        cx.notify();
    }

    /// Moves keyboard focus to the field.
    pub fn focus(&self, window: &mut Window) {
        window.focus(&self.focus_handle);
    }

    fn changed(&mut self, cx: &mut Context<Self>) {
        cx.emit(TextInputEvent::Changed(self.buffer.text().to_owned()));
        cx.notify();
    }

    fn offset_at(&self, position: Point<Pixels>) -> usize {
        let (Some(bounds), Some(line)) = (self.bounds.as_ref(), self.layout.as_ref()) else {
            return 0;
        };
        if position.y < bounds.top() {
            return 0;
        }
        if position.y > bounds.bottom() {
            return self.buffer.text().len();
        }
        line.closest_index_for_x(position.x - bounds.left())
    }

    fn step(&mut self, forward: bool, extend: bool, cx: &mut Context<Self>) {
        let from = self.buffer.cursor();
        let selection = self.buffer.selection();
        if !extend && !selection.is_empty() {
            self.buffer.move_to(if forward { selection.end } else { selection.start });
            return cx.notify();
        }
        let target = if forward { self.buffer.next_boundary(from) } else { self.buffer.previous_boundary(from) };
        if extend { self.buffer.select_to(target) } else { self.buffer.move_to(target) }
        cx.notify();
    }

    fn erase(&mut self, forward: bool, window: &mut Window, cx: &mut Context<Self>) {
        if self.buffer.selection().is_empty() {
            let from = self.buffer.cursor();
            let target = if forward { self.buffer.next_boundary(from) } else { self.buffer.previous_boundary(from) };
            self.buffer.select_to(target);
        }
        self.replace_text_in_range(None, "", window, cx);
    }

    fn on_mouse_down(&mut self, event: &MouseDownEvent, _: &mut Window, cx: &mut Context<Self>) {
        self.selecting = true;
        let offset = self.offset_at(event.position);
        if event.modifiers.shift { self.buffer.select_to(offset) } else { self.buffer.move_to(offset) }
        cx.notify();
    }

    fn on_mouse_move(&mut self, event: &MouseMoveEvent, _: &mut Window, cx: &mut Context<Self>) {
        if self.selecting {
            self.buffer.select_to(self.offset_at(event.position));
            cx.notify();
        }
    }

    fn on_mouse_up(&mut self, _: &MouseUpEvent, _: &mut Window, _: &mut Context<Self>) {
        self.selecting = false;
    }

    fn paste(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let Some(text) = cx.read_from_clipboard().and_then(|item| item.text()) else {
            return;
        };
        self.replace_text_in_range(None, &text.replace('\n', " "), window, cx);
    }

    fn copy(&self, cx: &mut Context<Self>) {
        if !self.buffer.selection().is_empty() {
            cx.write_to_clipboard(ClipboardItem::new_string(self.buffer.selected_text().to_owned()));
        }
    }
}

fn chrome(palette: &Palette, appearance: Appearance, focused: bool) -> (gpui::Hsla, gpui::Hsla) {
    let fill = match appearance {
        Appearance::Light => palette.background(),
        Appearance::Dark => palette.input().opacity(0.32),
    };
    (fill, if focused { palette.ring() } else { palette.input() })
}

impl Render for TextInput {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let focused = self.focus_handle.is_focused(window);
        let (fill, border) = chrome(palette, theme.appearance(), focused);
        let mut shadows = shadow::xs(0.05);
        if focused {
            shadows.push(gpui::BoxShadow {
                color: palette.ring().opacity(0.24),
                offset: gpui::point(px(0.0), px(0.0)),
                blur_radius: px(0.0),
                spread_radius: px(RING_WIDTH),
            });
        }
        div()
            .id("ui-text-input")
            .key_context(actions::CONTEXT)
            .track_focus(&self.focus_handle)
            .on_action(cx.listener(|this, _: &actions::Backspace, window, cx| this.erase(false, window, cx)))
            .on_action(cx.listener(|this, _: &actions::Delete, window, cx| this.erase(true, window, cx)))
            .on_action(cx.listener(|this, _: &actions::Left, _, cx| this.step(false, false, cx)))
            .on_action(cx.listener(|this, _: &actions::Right, _, cx| this.step(true, false, cx)))
            .on_action(cx.listener(|this, _: &actions::SelectLeft, _, cx| this.step(false, true, cx)))
            .on_action(cx.listener(|this, _: &actions::SelectRight, _, cx| this.step(true, true, cx)))
            .on_action(cx.listener(|this, _: &actions::SelectAll, _, cx| {
                this.buffer.select_all();
                cx.notify();
            }))
            .on_action(cx.listener(|this, _: &actions::Home, _, cx| {
                this.buffer.move_to(0);
                cx.notify();
            }))
            .on_action(cx.listener(|this, _: &actions::End, _, cx| {
                this.buffer.move_to(this.buffer.text().len());
                cx.notify();
            }))
            .on_action(cx.listener(|this, _: &actions::Paste, window, cx| this.paste(window, cx)))
            .on_action(cx.listener(|this, _: &actions::Copy, _, cx| this.copy(cx)))
            .on_action(cx.listener(|this, _: &actions::Cut, window, cx| {
                this.copy(cx);
                this.replace_text_in_range(None, "", window, cx);
            }))
            .on_action(cx.listener(|_, _: &actions::Submit, _, cx| cx.emit(TextInputEvent::Submitted)))
            .on_action(cx.listener(|_, _: &actions::Cancel, _, cx| cx.emit(TextInputEvent::Cancelled)))
            .on_mouse_down(MouseButton::Left, cx.listener(Self::on_mouse_down))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::on_mouse_up))
            .on_mouse_move(cx.listener(Self::on_mouse_move))
            .cursor(CursorStyle::IBeam)
            .flex()
            .items_center()
            .w_full()
            .h(px(CONTROL_HEIGHT))
            .px(px(PADDING))
            .rounded(radius::LG)
            .border_1()
            .border_color(border)
            .bg(fill)
            .text_color(palette.foreground())
            .text_size(text_size::SM)
            .line_height(px(20.0))
            .font_family(UI_FAMILY)
            .shadow(shadows)
            .child(TextInputElement::new(cx.entity(), palette))
    }
}

impl EntityInputHandler for TextInput {
    fn text_for_range(
        &mut self,
        range_utf16: Range<usize>,
        actual_range: &mut Option<Range<usize>>,
        _: &mut Window,
        _: &mut Context<Self>,
    ) -> Option<String> {
        let range = self.buffer.range_from_utf16(&range_utf16);
        actual_range.replace(self.buffer.range_to_utf16(&range));
        self.buffer.text().get(range).map(str::to_owned)
    }

    fn selected_text_range(&mut self, _: bool, _: &mut Window, _: &mut Context<Self>) -> Option<UTF16Selection> {
        Some(UTF16Selection { range: self.buffer.range_to_utf16(&self.buffer.selection()), reversed: self.buffer.is_reversed() })
    }

    fn marked_text_range(&self, _: &mut Window, _: &mut Context<Self>) -> Option<Range<usize>> {
        self.buffer.marked().map(|range| self.buffer.range_to_utf16(&range))
    }

    fn unmark_text(&mut self, _: &mut Window, _: &mut Context<Self>) {
        self.buffer.unmark();
    }

    fn replace_text_in_range(&mut self, range: Option<Range<usize>>, text: &str, _: &mut Window, cx: &mut Context<Self>) {
        let range = range.map(|range| self.buffer.range_from_utf16(&range));
        self.buffer.replace(range, text);
        self.changed(cx);
    }

    fn replace_and_mark_text_in_range(
        &mut self,
        range: Option<Range<usize>>,
        text: &str,
        selected: Option<Range<usize>>,
        _: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let range = range.map(|range| self.buffer.range_from_utf16(&range));
        let selected = selected.map(|range| self.buffer.range_from_utf16(&range));
        self.buffer.replace_and_mark(range, text, selected);
        self.changed(cx);
    }

    fn bounds_for_range(&mut self, range: Range<usize>, bounds: Bounds<Pixels>, _: &mut Window, _: &mut Context<Self>) -> Option<Bounds<Pixels>> {
        let line = self.layout.as_ref()?;
        let range = self.buffer.range_from_utf16(&range);
        Some(Bounds::from_corners(
            gpui::point(bounds.left() + line.x_for_index(range.start), bounds.top()),
            gpui::point(bounds.left() + line.x_for_index(range.end), bounds.bottom()),
        ))
    }

    fn character_index_for_point(&mut self, point: Point<Pixels>, _: &mut Window, _: &mut Context<Self>) -> Option<usize> {
        let local = self.bounds?.localize(&point)?;
        let line = self.layout.as_ref()?;
        let index = line.index_for_x(local.x)?;
        Some(self.buffer.to_utf16(index))
    }
}

impl TextInput {
    /// Builds a field entity in one call.
    pub fn build(placeholder: impl Into<SharedString>, cx: &mut App) -> Entity<Self> {
        cx.new(|cx| Self::new(placeholder, cx))
    }
}

#[cfg(test)]
#[path = "text_input_tests.rs"]
mod tests;
