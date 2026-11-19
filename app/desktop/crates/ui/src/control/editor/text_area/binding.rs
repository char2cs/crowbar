//! Wires the keyboard actions of [`TextArea`] to its commands.

mod actions;
mod caret_motion;
mod paging;

pub(crate) use actions::{CONTEXT, bindings};

use gpui::{Context, Div, InteractiveElement, Stateful};

use crate::control::editor::text_area::TextArea;
use crate::control::editor::text_area::binding::actions as area;
use crate::control::editor::text_input::actions as shared;
use crate::control::editor::text_input_event::TextInputEvent;
use crate::foundation::Direction;

impl TextArea {
    pub(crate) fn bind_actions(
        field: Stateful<Div>,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        Self::bind_selection(Self::bind_editing(Self::bind_movement(field, cx), cx), cx)
            .on_action(cx.listener(|this, _: &area::Newline, _, cx| this.insert("\n", cx)))
            .on_action(cx.listener(|this, _: &shared::Submit, _, cx| {
                this.history.break_typing();
                cx.emit(TextInputEvent::Submitted);
            }))
            .on_action(
                cx.listener(|_, _: &shared::Cancel, _, cx| cx.emit(TextInputEvent::Cancelled)),
            )
    }

    fn bind_movement(
        field: Stateful<Div>,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        field
            .on_action(cx.listener(|this, _: &shared::Left, _, cx| {
                this.move_caret(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::Right, _, cx| {
                this.move_caret(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::WordLeft, _, cx| {
                this.move_by_word(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::WordRight, _, cx| {
                this.move_by_word(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::PageUp, _, cx| {
                this.page(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::PageDown, _, cx| {
                this.page(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::Up, _, cx| {
                this.move_vertically(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::Down, _, cx| {
                this.move_vertically(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::Home, _, cx| {
                this.move_to_row_edge(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::End, _, cx| {
                this.move_to_row_edge(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::DocumentStart, _, cx| {
                this.move_to_document_edge(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::DocumentEnd, _, cx| {
                this.move_to_document_edge(Direction::Forward, cx);
            }))
    }

    fn bind_selection(
        field: Stateful<Div>,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        field
            .on_action(cx.listener(|this, _: &shared::SelectLeft, _, cx| {
                this.extend_selection(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::SelectRight, _, cx| {
                this.extend_selection(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::SelectUp, _, cx| {
                this.extend_vertically(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::SelectDown, _, cx| {
                this.extend_vertically(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::SelectHome, _, cx| {
                this.extend_to_row_edge(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::SelectEnd, _, cx| {
                this.extend_to_row_edge(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::SelectWordLeft, _, cx| {
                this.extend_by_word(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::SelectWordRight, _, cx| {
                this.extend_by_word(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::SelectPageUp, _, cx| {
                this.extend_page(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &area::SelectPageDown, _, cx| {
                this.extend_page(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::SelectAll, _, cx| {
                this.buffer.select_all();
                this.restart_blink(cx);
            }))
    }

    fn bind_editing(
        field: Stateful<Div>,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        field
            .on_action(cx.listener(|this, _: &shared::Backspace, _, cx| {
                this.erase(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::Delete, _, cx| {
                this.erase(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::DeleteWordLeft, _, cx| {
                this.erase_word(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::DeleteWordRight, _, cx| {
                this.erase_word(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &shared::Paste, _, cx| this.paste(cx)))
            .on_action(cx.listener(|this, _: &shared::Copy, _, cx| this.copy(cx)))
            .on_action(cx.listener(|this, _: &shared::Cut, _, cx| this.cut(cx)))
            .on_action(cx.listener(|this, _: &area::Undo, _, cx| this.undo(cx)))
            .on_action(cx.listener(|this, _: &area::Redo, _, cx| this.redo(cx)))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::TestAppContext;

    use crate::TextInputEvent;
    use crate::control::editor::test_support::area::{area, area_text};

    #[gpui::test]
    fn plain_enter_adds_a_line_and_the_modifier_submits(cx: &mut TestAppContext) {
        let (area, cx) = area(cx);
        let (submitted, cancelled) = (Rc::new(Cell::new(0)), Rc::new(Cell::new(0)));
        let (on_submit, on_cancel) = (Rc::clone(&submitted), Rc::clone(&cancelled));
        cx.update(|_, cx| {
            cx.subscribe(&area, move |_, event: &TextInputEvent, _| match event {
                TextInputEvent::Submitted => on_submit.set(on_submit.get() + 1),
                TextInputEvent::Cancelled => on_cancel.set(on_cancel.get() + 1),
                TextInputEvent::Changed(_) => {},
            })
            .detach();
        });
        cx.simulate_input("a");
        cx.simulate_keystrokes("enter");
        assert_eq!((submitted.get(), area_text(&area, cx)), (0, "a\n".into()));
        cx.simulate_keystrokes("secondary-enter escape");
        assert_eq!(
            (submitted.get(), cancelled.get(), area_text(&area, cx)),
            (1, 1, "a\n".into())
        );
    }
}
