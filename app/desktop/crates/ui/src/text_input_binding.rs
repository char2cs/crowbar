//! Wires the keyboard actions of [`TextInput`] to its commands.

use gpui::{Context, Div, InteractiveElement, Stateful};

use crate::direction::Direction;
use crate::text_input::TextInput;
use crate::text_input_actions as actions;
use crate::text_input_event::TextInputEvent;

impl TextInput {
    pub(crate) fn bind_actions(field: Stateful<Div>, cx: &mut Context<'_, Self>) -> Stateful<Div> {
        Self::bind_editing(Self::bind_movement(field, cx), cx)
            .on_action(
                cx.listener(|_, _: &actions::Submit, _, cx| cx.emit(TextInputEvent::Submitted)),
            )
            .on_action(
                cx.listener(|_, _: &actions::Cancel, _, cx| cx.emit(TextInputEvent::Cancelled)),
            )
    }

    fn bind_movement(field: Stateful<Div>, cx: &mut Context<'_, Self>) -> Stateful<Div> {
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
    }

    fn bind_editing(field: Stateful<Div>, cx: &mut Context<'_, Self>) -> Stateful<Div> {
        field
            .on_action(cx.listener(|this, _: &actions::Backspace, window, cx| {
                this.erase(Direction::Backward, window, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::Delete, window, cx| {
                this.erase(Direction::Forward, window, cx);
            }))
            .on_action(cx.listener(|this, _: &actions::Paste, window, cx| this.paste(window, cx)))
            .on_action(cx.listener(|this, _: &actions::Copy, _, cx| this.copy(cx)))
            .on_action(cx.listener(|this, _: &actions::Cut, window, cx| this.cut(window, cx)))
    }
}

#[cfg(test)]
#[path = "text_input_binding_tests.rs"]
mod tests;
