use gpui::{
    AppContext as _, Context, Entity, EventEmitter, IntoElement, ParentElement, Render, Styled,
    Window, div,
};
use ui::{Button, ButtonVariant, Dialog, DialogSize, Surface, TextInput, TextInputEvent};

use crate::rename_event::RenameEvent;

const BUTTON_GAP: f32 = 8.0;
const PLACEHOLDER: &str = "Name";

pub(crate) struct RenamePrompt {
    field: Entity<TextInput>,
    _watch: gpui::Subscription,
}

impl EventEmitter<RenameEvent> for RenamePrompt {}

impl RenamePrompt {
    pub(crate) fn new(
        current: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let field = cx.new(|cx| TextInput::new(PLACEHOLDER, cx).on(Surface::Window));
        field.update(cx, |field, cx| field.set_text(current, cx));
        field.read(cx).focus(window);
        let watch = cx.subscribe(&field, |this, _, event: &TextInputEvent, cx| match event {
            TextInputEvent::Submitted => this.confirm(cx),
            TextInputEvent::Cancelled => Self::dismiss(cx),
            _ => {},
        });
        Self {
            field,
            _watch: watch,
        }
    }

    fn confirm(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let name = self.field.read(cx).text().trim().to_owned();
        cx.emit(RenameEvent::Confirmed(name));
    }

    fn dismiss(cx: &mut Context<'_, Self>) {
        cx.emit(RenameEvent::Dismissed);
    }
}

impl Render for RenamePrompt {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let cancel = Button::new("rename-prompt-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .on_click(cx.listener(|_, _, _, cx| Self::dismiss(cx)));
        let rename = Button::new("rename-prompt-confirm", "Rename")
            .variant(ButtonVariant::Secondary)
            .on_click(cx.listener(|this, _, _, cx| this.confirm(cx)));
        let prompt = cx.entity();
        Dialog::new("rename-prompt")
            .title("Rename Tab")
            .size(DialogSize::Sm)
            .body(self.field.clone())
            .footer(
                div()
                    .flex()
                    .gap(gpui::px(BUTTON_GAP))
                    .child(cancel)
                    .child(rename),
            )
            .on_close(move |_, cx| prompt.update(cx, |_, cx| RenamePrompt::dismiss(cx)))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::*;

    type Heard = Rc<RefCell<Vec<RenameEvent>>>;

    fn recorder(
        sink: &Heard
    ) -> impl FnMut(Entity<RenamePrompt>, &RenameEvent, &mut gpui::App) + 'static {
        let sink = Rc::clone(sink);
        move |_, event, _| sink.borrow_mut().push(event.clone())
    }

    fn prompt(
        cx: &mut TestAppContext,
        current: &str,
    ) -> (Entity<RenamePrompt>, Heard) {
        cx.update(|cx| cx.set_global(ui::Theme::default()));
        let (prompt, cx) = cx.add_window_view(|window, cx| RenamePrompt::new(current, window, cx));
        let heard = Heard::default();
        cx.update(|_, cx| cx.subscribe(&prompt, recorder(&heard)).detach());
        (prompt, heard)
    }

    const CASES: &[(&str, &str, &str)] = &[
        ("the name is sent as typed", "logs", "logs"),
        ("surrounding blanks are dropped", "  logs  ", "logs"),
        ("an emptied field asks for the default name", "   ", ""),
    ];

    #[gpui::test]
    fn confirming_sends_the_trimmed_name(cx: &mut TestAppContext) {
        for (name, typed, sent) in CASES {
            let (prompt, heard) = prompt(cx, typed);
            prompt.update(cx, RenamePrompt::confirm);
            assert_eq!(
                *heard.borrow(),
                [RenameEvent::Confirmed((*sent).to_owned())],
                "{name}"
            );
        }
    }

    #[gpui::test]
    fn dismissing_sends_no_name(cx: &mut TestAppContext) {
        let (prompt, heard) = prompt(cx, "zsh");
        prompt.update(cx, |_, cx| RenamePrompt::dismiss(cx));
        assert_eq!(*heard.borrow(), [RenameEvent::Dismissed]);
    }
}
