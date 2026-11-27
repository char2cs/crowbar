use gpui::{
    AppContext as _, Context, Entity, EventEmitter, IntoElement, ParentElement, Render, Styled,
    Window, div,
};
use ui::{
    Button, ButtonVariant, Dialog, DialogSize, DialogStyle, Surface, TextInput, TextInputEvent,
};

use crate::shell::dialogs::name_event::NameEvent;
use crate::shell::dialogs::prefill::Prefill;
use crate::shell::dialogs::prompt_words::PromptWords;

const BUTTON_GAP: f32 = 8.0;
const LABEL_GAP: f32 = 8.0;

pub(crate) const RENAME: PromptWords = PromptWords {
    dialog: "rename-prompt",
    title: "Rename Terminal",
    label: "Enter a terminal name:",
    placeholder: "Terminal name",
    cancel_id: "rename-prompt-cancel",
    confirm_id: "rename-prompt-confirm",
    confirm: "Rename",
    prefill: Prefill::CaretAtEnd,
};

pub(crate) const SAVE_AS: PromptWords = PromptWords {
    dialog: "save-as-prompt",
    title: "Save as:",
    label: "",
    placeholder: "Name",
    cancel_id: "save-as-prompt-cancel",
    confirm_id: "save-as-prompt-confirm",
    confirm: "OK",
    prefill: Prefill::Selected,
};

pub(crate) struct NamePrompt {
    words: PromptWords,
    field: Entity<TextInput>,
    _watch: gpui::Subscription,
}

impl EventEmitter<NameEvent> for NamePrompt {}

impl NamePrompt {
    pub(crate) fn new(
        (current, words): (&str, PromptWords),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let field = cx.new(|cx| TextInput::new(words.placeholder, cx).on(Surface::Window));
        field.update(cx, |field, cx| field.set_text(current, cx));
        if words.prefill == Prefill::Selected {
            field.update(cx, TextInput::select_all);
        }
        field.update(cx, |field, cx| field.focus(window, cx));
        let watch = cx.subscribe(&field, |this, _, event: &TextInputEvent, cx| match event {
            TextInputEvent::Submitted => this.confirm(cx),
            TextInputEvent::Cancelled => Self::dismiss(cx),
            _ => {},
        });
        Self {
            words,
            field,
            _watch: watch,
        }
    }

    fn confirm(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let name = self.field.read(cx).text().trim().to_owned();
        cx.emit(NameEvent::Confirmed(name));
    }

    fn dismiss(cx: &mut Context<'_, Self>) {
        cx.emit(NameEvent::Dismissed);
    }
}

impl NamePrompt {
    fn body(
        &self,
        cx: &Context<'_, Self>,
    ) -> gpui::Div {
        let label = (!self.words.label.is_empty()).then(|| {
            let palette = ui::Theme::of(cx).palette(Surface::Window);
            div()
                .text_size(ui::text_size::SM)
                .text_color(palette.foreground())
                .child(self.words.label)
        });
        div()
            .flex()
            .flex_col()
            .gap(gpui::px(LABEL_GAP))
            .children(label)
            .child(self.field.clone())
    }
}

impl Render for NamePrompt {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let cancel = Button::new(self.words.cancel_id, "Cancel")
            .variant(ButtonVariant::Outline)
            .on_click(cx.listener(|_, _, _, cx| Self::dismiss(cx)));
        let rename = Button::new(self.words.confirm_id, self.words.confirm)
            .on_click(cx.listener(|this, _, _, cx| this.confirm(cx)));
        let prompt = cx.entity();
        Dialog::new(self.words.dialog)
            .title(self.words.title)
            .style(DialogStyle::Plain)
            .size(DialogSize::Base)
            .body(self.body(cx))
            .footer(
                div()
                    .flex()
                    .gap(gpui::px(BUTTON_GAP))
                    .child(cancel)
                    .child(rename),
            )
            .on_close(move |_, cx| prompt.update(cx, |_, cx| NamePrompt::dismiss(cx)))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::*;

    type Heard = Rc<RefCell<Vec<NameEvent>>>;

    fn recorder(
        sink: &Heard
    ) -> impl FnMut(Entity<NamePrompt>, &NameEvent, &mut gpui::App) + 'static {
        let sink = Rc::clone(sink);
        move |_, event, _| sink.borrow_mut().push(event.clone())
    }

    fn prompt(
        cx: &mut TestAppContext,
        current: &str,
    ) -> (Entity<NamePrompt>, Heard) {
        cx.update(|cx| cx.set_global(ui::Theme::default()));
        let (prompt, cx) =
            cx.add_window_view(|window, cx| NamePrompt::new((current, RENAME), window, cx));
        let heard = Heard::default();
        cx.update(|_, cx| cx.subscribe(&prompt, recorder(&heard)).detach());
        (prompt, heard)
    }

    const CASES: &[(&str, &str, &str)] = &[
        ("the name is sent as typed", "logs", "logs"),
        ("surrounding blanks are dropped", "  logs  ", "logs"),
        ("an emptied field sends an empty name", "   ", ""),
    ];

    #[gpui::test]
    fn confirming_sends_the_trimmed_name(cx: &mut TestAppContext) {
        for (name, typed, sent) in CASES {
            let (prompt, heard) = prompt(cx, typed);
            prompt.update(cx, NamePrompt::confirm);
            assert_eq!(
                *heard.borrow(),
                [NameEvent::Confirmed((*sent).to_owned())],
                "{name}"
            );
        }
    }

    #[gpui::test]
    fn dismissing_sends_no_name(cx: &mut TestAppContext) {
        let (prompt, heard) = prompt(cx, "zsh");
        prompt.update(cx, |_, cx| NamePrompt::dismiss(cx));
        assert_eq!(*heard.borrow(), [NameEvent::Dismissed]);
    }

    #[gpui::test]
    fn typing_replaces_a_selected_prefill_and_follows_one_left_at_its_end(cx: &mut TestAppContext) {
        cx.update(|cx| cx.set_global(ui::Theme::default()));
        let cases = [(RENAME, "Terminalx"), (SAVE_AS, "x")];
        for (words, expected) in cases {
            let (prompt, window) =
                cx.add_window_view(|window, cx| NamePrompt::new(("Terminal", words), window, cx));
            window.simulate_input("x");
            let typed = window.read(|cx| prompt.read(cx).field.read(cx).text().to_owned());
            assert_eq!(typed, expected, "{}", words.title);
        }
    }

    #[gpui::test]
    fn the_prompt_is_develops_plain_dialog_at_its_full_width(cx: &mut TestAppContext) {
        cx.update(|cx| cx.set_global(ui::Theme::default()));
        let (_, window) =
            cx.add_window_view(|window, cx| NamePrompt::new(("Terminal", RENAME), window, cx));
        window.run_until_parked();
        let shown = window
            .debug_bounds("ui-dialog")
            .map(|bounds| bounds.size.width);
        assert_eq!(shown, Some(gpui::px(512.0)), "max-w-lg");
    }
}
