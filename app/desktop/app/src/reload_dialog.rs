use gpui::{Context, EventEmitter, IntoElement, ParentElement, Render, Styled, Window, div};
use protocol::PaneId;
use ui::{Button, ButtonVariant, Dialog, DialogSize, DialogStyle};

use crate::close_choice::CloseChoice;

const BUTTON_GAP: f32 = 8.0;

pub(crate) struct ReloadDialog {
    pane: PaneId,
    message: String,
}

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/tabs/components/tab-bar-context-menu.tsx
pub(crate) fn message_for(name: &str) -> String {
    format!("Discard unsaved changes to {name} and reload it from disk?")
}

impl EventEmitter<CloseChoice> for ReloadDialog {}

impl ReloadDialog {
    pub(crate) fn new((pane, name): (PaneId, String)) -> Self {
        Self {
            pane,
            message: message_for(&name),
        }
    }

    pub(crate) const fn pane(&self) -> &PaneId {
        &self.pane
    }
}

impl Render for ReloadDialog {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let choose = |choice: CloseChoice| cx.listener(move |_, _, _, cx| cx.emit(choice));
        let cancel = Button::new("reload-cancel", "Cancel")
            .variant(ButtonVariant::Ghost)
            .on_click(choose(CloseChoice::Cancel));
        let discard = Button::new("reload-ok", "OK").on_click(choose(CloseChoice::Discard));
        let dialog = cx.entity();
        Dialog::new("reload-discard")
            .style(DialogStyle::Alert)
            .description(self.message.clone())
            .size(DialogSize::Sm)
            .footer(
                div()
                    .flex()
                    .gap(gpui::px(BUTTON_GAP))
                    .child(cancel)
                    .child(discard),
            )
            .on_close(move |_, cx| dialog.update(cx, |_, cx| cx.emit(CloseChoice::Cancel)))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_question_names_the_file() {
        assert_eq!(
            message_for("README.md"),
            "Discard unsaved changes to README.md and reload it from disk?"
        );
    }
}
