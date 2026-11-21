use gpui::{AppContext as _, Context, Entity, Window};
use layout::Command;
use protocol::{PaneId, ShellFrame};

use super::ShellView;
use crate::close_choice::CloseChoice;
use crate::close_guard::dirty_among;
use crate::close_step::CloseStep;
use crate::closing::Closing;
use crate::document_request::DocumentRequest;
use crate::pending_close::PendingClose;
use crate::request_queue::send;
use crate::shell_request::ShellRequest;
use crate::titles::Titles;
use crate::unsaved_dialog::UnsavedDialog;

pub(super) fn names_of(
    panes: &[PaneId],
    titles: &Titles,
) -> Vec<String> {
    panes
        .iter()
        .map(|pane| {
            titles
                .get(pane)
                .map_or_else(|| pane.to_string(), |title| title.to_string())
        })
        .collect()
}

impl ShellView {
    pub(super) fn apply_guarded(
        &mut self,
        commands: Vec<Command>,
        cx: &mut Context<'_, Self>,
    ) {
        let flags = self.state.dirty();
        let dirty = dirty_among(self.state.layout(), &commands, &flags);
        if dirty.is_empty() {
            self.state.apply(commands);
            self.refresh(cx);
            return;
        }
        self.ask_before_closing(PendingClose::new(Closing::Panes(commands), dirty), cx);
    }

    pub(crate) fn allow_window_close(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        let dirty = self.state.dirty().all();
        if dirty.is_empty() {
            return true;
        }
        if self.pending_close.is_none() {
            self.ask_before_closing(PendingClose::new(Closing::Window, dirty), cx);
        }
        false
    }

    fn ask_before_closing(
        &mut self,
        pending: PendingClose,
        cx: &mut Context<'_, Self>,
    ) {
        let names = names_of(pending.dirty(), &self.state.titles());
        let dialog = cx.new(|_| UnsavedDialog::new(&names));
        self.subscriptions.push(
            cx.subscribe(&dialog, |this, source, choice: &CloseChoice, cx| {
                this.on_unsaved_choice(source, *choice, cx);
            }),
        );
        self.unsaved = Some(dialog);
        self.pending_close = Some(pending);
        cx.notify();
    }

    fn on_unsaved_choice(
        &mut self,
        _: Entity<UnsavedDialog>,
        choice: CloseChoice,
        _: &mut Context<'_, Self>,
    ) {
        send(&self.requests, ShellRequest::Closing(choice));
    }

    pub(super) fn choose_closing(
        &mut self,
        choice: CloseChoice,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.unsaved = None;
        let Some(pending) = self.pending_close.take() else {
            return cx.notify();
        };
        if choice == CloseChoice::Save {
            self.request_saves(pending.dirty());
        }
        let step = pending.choose(choice, &self.state.dirty());
        self.advance(step, window, cx);
    }

    pub(super) fn resume_closing(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(pending) = self.pending_close.take() else {
            return;
        };
        let step = pending.settle(&self.state.dirty());
        self.advance(step, window, cx);
    }

    fn advance(
        &mut self,
        step: CloseStep<PendingClose>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match step {
            CloseStep::Run(Closing::Panes(commands)) => {
                self.state.apply(commands);
                self.refresh(cx);
            },
            CloseStep::Run(Closing::Window) => window.remove_window(),
            CloseStep::Wait(pending) => self.pending_close = Some(pending),
            CloseStep::Stop => {},
        }
        cx.notify();
    }

    fn request_saves(
        &self,
        panes: &[PaneId],
    ) {
        for pane in panes {
            self.request_save(pane);
        }
    }

    fn request_save(
        &self,
        pane: &PaneId,
    ) {
        if self.source.native().editors().contains(pane) {
            return send(
                &self.requests,
                ShellRequest::Document(DocumentRequest::Save { pane: pane.clone() }),
            );
        }
        if let Err(error) = self.state.host().send(pane, &ShellFrame::Save) {
            tracing::warn!(%error, %pane, "asking a pane to save failed");
        }
    }
}

#[cfg(test)]
mod tests {
    use protocol::PaneTitle;

    use super::*;

    #[test]
    fn a_pane_is_named_by_its_title_or_by_its_id_before_it_has_one() {
        let titles = Titles::default();
        let (titled, bare) = (PaneId::new("p1").unwrap(), PaneId::new("p2").unwrap());
        titles.set(titled.clone(), PaneTitle::new("a.rs").unwrap());
        assert_eq!(names_of(&[titled, bare], &titles), ["a.rs", "p2"]);
    }
}
