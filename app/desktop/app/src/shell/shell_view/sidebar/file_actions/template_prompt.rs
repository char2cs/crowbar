use gpui::{AppContext as _, Context, ParentElement as _, Styled as _, Window};
use store::TemplateOutcome;

use super::ShellView;
use super::template_request::TemplateRequest;
use crate::shell::dialogs::file_prompt::FilePrompt;
use crate::shell::dialogs::pending_file::PendingFile;
use crate::shell::dialogs::prompt_event::PromptEvent;
use crate::shell::shell_view::sidebar::file_reply;

impl ShellView {
    pub(super) fn make_env_template(
        &mut self,
        request: &TemplateRequest<'_>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let TemplateRequest {
            source,
            target,
            overwrite,
        } = *request;
        let (done, reply) = file_reply::channel();
        let made = self
            .data
            .commands()
            .create_env_template((source, target), overwrite, done);
        if let Err(error) = made {
            return self.fail(error.to_string(), cx);
        }
        let pending = PendingFile::Overwrite {
            path: source.to_owned(),
            target: target.to_owned(),
        };
        let then =
            move |this: &mut Self, outcome, window: &mut Window, cx: &mut Context<'_, Self>| {
                this.ask_if(confirmation_for(&outcome, pending), window, cx);
            };
        Self::when_answered(reply, then, (window, cx));
    }

    pub(in crate::shell::shell_view) fn prompt_layer(&self) -> Option<gpui::Div> {
        let prompt = self.file_prompt.clone()?;
        Some(gpui::div().absolute().size_full().child(prompt))
    }

    fn ask_if(
        &mut self,
        pending: Option<PendingFile>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(pending) = pending else {
            return;
        };
        self.ask(pending, window, cx);
    }

    pub(super) fn ask(
        &mut self,
        pending: PendingFile,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.file_prompt.is_some() {
            return;
        }
        let prompt = cx.new(|_| FilePrompt::new(pending));
        self.subscriptions.push(
            cx.subscribe_in(&prompt, window, |this, _, event, window, cx| {
                this.on_file_prompt(event, window, cx);
            }),
        );
        self.file_prompt = Some(prompt);
        cx.notify();
    }

    fn on_file_prompt(
        &mut self,
        event: &PromptEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.file_prompt = None;
        cx.notify();
        let PromptEvent::Confirmed(pending) = event else {
            return;
        };
        match pending {
            PendingFile::OpenAll(files) => self.open_each(files, cx),
            PendingFile::Overwrite { path, target } => {
                let request = TemplateRequest::replacing(path, target);
                self.make_env_template(&request, window, cx);
            },
        }
    }
}

fn confirmation_for(
    outcome: &TemplateOutcome,
    pending: PendingFile,
) -> Option<PendingFile> {
    matches!(outcome, TemplateOutcome::Exists(_)).then_some(pending)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_an_existing_template_asks_before_it_is_replaced() {
        let pending = || PendingFile::OpenAll(vec!["a".into()]);
        let created = TemplateOutcome::Created(".env.example".into());
        let exists = TemplateOutcome::Exists(".env.example".into());
        assert!(confirmation_for(&created, pending()).is_none(), "created");
        assert!(confirmation_for(&exists, pending()).is_some(), "exists");
    }
}
