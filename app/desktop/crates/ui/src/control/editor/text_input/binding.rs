//! Wires the keyboard actions of [`TextInput`] to its commands.

use gpui::{Context, Div, InteractiveElement, Stateful};

use crate::control::editor::text_input::TextInput;
use crate::control::editor::text_input::actions;
use crate::control::editor::text_input_event::TextInputEvent;
use crate::foundation::Direction;

impl TextInput {
    pub(crate) fn bind_actions(
        field: Stateful<Div>,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        Self::bind_editing(Self::bind_movement(field, cx), cx)
            .on_action(
                cx.listener(|_, _: &actions::Submit, _, cx| cx.emit(TextInputEvent::Submitted)),
            )
            .on_action(
                cx.listener(|_, _: &actions::Cancel, _, cx| cx.emit(TextInputEvent::Cancelled)),
            )
    }

    fn bind_movement(
        field: Stateful<Div>,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        field
            .on_action(cx.listener(|this, _: &actions::Left, _, cx| {
                this.move_caret(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::Right, _, cx| {
                this.move_caret(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::SelectLeft, _, cx| {
                this.extend_selection(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::SelectRight, _, cx| {
                this.extend_selection(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::WordLeft, _, cx| {
                this.move_by_word(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::WordRight, _, cx| {
                this.move_by_word(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::SelectWordLeft, _, cx| {
                this.extend_by_word(Direction::Backward, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::SelectWordRight, _, cx| {
                this.extend_by_word(Direction::Forward, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::SelectAll, _, cx| {
                this.buffer.select_all();
                this.restart_blink(cx);
            }))
            .on_action(cx.listener(|this, _: &actions::Home, _, cx| {
                this.buffer.move_to(0);
                this.restart_blink(cx);
            }))
            .on_action(cx.listener(|this, _: &actions::End, _, cx| {
                this.buffer.move_to(this.buffer.text().len());
                this.restart_blink(cx);
            }))
    }

    fn bind_editing(
        field: Stateful<Div>,
        cx: &mut Context<'_, Self>,
    ) -> Stateful<Div> {
        field
            .on_action(cx.listener(|this, _: &actions::Backspace, window, cx| {
                this.erase(Direction::Backward, window, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::Delete, window, cx| {
                this.erase(Direction::Forward, window, cx);
            }))
            .on_action(
                cx.listener(|this, _: &actions::DeleteWordLeft, window, cx| {
                    this.erase_word(Direction::Backward, window, cx);
                }),
            )
            .on_action(
                cx.listener(|this, _: &actions::DeleteWordRight, window, cx| {
                    this.erase_word(Direction::Forward, window, cx);
                }),
            )
            .on_action(cx.listener(|this, _: &actions::Paste, window, cx| this.paste(window, cx)))
            .on_action(cx.listener(|this, _: &actions::Copy, _, cx| this.copy(cx)))
            .on_action(cx.listener(|this, _: &actions::Cut, window, cx| this.cut(window, cx)))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::TestAppContext;

    use crate::TextInputEvent;
    use crate::control::editor::test_support::field::field;

    #[gpui::test]
    fn enter_submits_and_escape_cancels(cx: &mut TestAppContext) {
        let (input, cx) = field(cx);
        let (submitted, cancelled) = (Rc::new(Cell::new(0)), Rc::new(Cell::new(0)));
        let (on_submit, on_cancel) = (Rc::clone(&submitted), Rc::clone(&cancelled));
        cx.update(|_, cx| {
            cx.subscribe(&input, move |_, event: &TextInputEvent, _| match event {
                TextInputEvent::Submitted => on_submit.set(on_submit.get() + 1),
                TextInputEvent::Cancelled => on_cancel.set(on_cancel.get() + 1),
                TextInputEvent::Changed(_) => {},
            })
            .detach();
        });
        cx.simulate_keystrokes("enter escape escape");
        assert_eq!((submitted.get(), cancelled.get()), (1, 2));
    }
}
