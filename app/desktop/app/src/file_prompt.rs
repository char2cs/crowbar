use gpui::{Context, EventEmitter, IntoElement, ParentElement, Render, Styled, Window, div};
use ui::{Button, ButtonVariant, Dialog, DialogSize, Surface, Theme};

use crate::pending_file::PendingFile;
use crate::prompt_event::PromptEvent;

const BUTTON_GAP: f32 = 8.0;

pub(crate) struct FilePrompt {
    pending: PendingFile,
}

fn tone_of(pending: &PendingFile) -> ButtonVariant {
    if pending.destroys() {
        return ButtonVariant::Destructive;
    }
    ButtonVariant::Secondary
}

impl EventEmitter<PromptEvent> for FilePrompt {}

impl FilePrompt {
    pub(crate) fn new(pending: PendingFile) -> Self {
        Self { pending }
    }

    fn confirm(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        cx.emit(PromptEvent::Confirmed(self.pending.clone()));
    }

    fn dismiss(cx: &mut Context<'_, Self>) {
        cx.emit(PromptEvent::Dismissed);
    }
}

impl Render for FilePrompt {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let cancel = Button::new("file-prompt-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .on_click(cx.listener(|_, _, _, cx| Self::dismiss(cx)));
        let tone = tone_of(&self.pending);
        let confirm = Button::new("file-prompt-confirm", self.pending.confirm_label())
            .variant(tone)
            .on_click(cx.listener(|this, _, _, cx| this.confirm(cx)));
        let prompt = cx.entity();
        Dialog::new("file-prompt")
            .title(self.pending.title())
            .size(DialogSize::Sm)
            .body(
                div()
                    .text_color(palette.foreground())
                    .child(self.pending.message()),
            )
            .footer(
                div()
                    .flex()
                    .gap(gpui::px(BUTTON_GAP))
                    .child(cancel)
                    .child(confirm),
            )
            .on_close(move |_, cx| prompt.update(cx, |_, cx| FilePrompt::dismiss(cx)))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    type Heard = Rc<RefCell<Vec<PromptEvent>>>;

    fn note(
        heard: &Heard,
        event: &PromptEvent,
    ) {
        heard.borrow_mut().push(event.clone());
    }

    fn prompt(cx: &mut TestAppContext) -> (gpui::Entity<FilePrompt>, Heard) {
        let pending = PendingFile::OpenAll(vec!["a.rs".into()]);
        let prompt = cx.new(|_| FilePrompt::new(pending));
        let heard = Heard::default();
        let sink = Rc::clone(&heard);
        cx.update(|cx| {
            cx.subscribe(&prompt, move |_, event: &PromptEvent, _| note(&sink, event))
                .detach();
        });
        (prompt, heard)
    }

    #[gpui::test]
    fn confirming_and_dismissing_each_say_so_once(cx: &mut TestAppContext) {
        let (prompt, heard) = prompt(cx);
        prompt.update(cx, |_, cx| FilePrompt::dismiss(cx));
        prompt.update(cx, FilePrompt::confirm);
        assert_eq!(
            *heard.borrow(),
            [
                PromptEvent::Dismissed,
                PromptEvent::Confirmed(PendingFile::OpenAll(vec!["a.rs".into()]))
            ]
        );
    }
}
