use std::path::PathBuf;

use gpui::{
    AppContext as _, AsyncWindowContext, Context, Entity, ParentElement as _, PathPromptOptions,
    Styled as _, WeakEntity, Window,
};
use shell_data::{Overwrite, TemplateOutcome};
use sidebar::Command;
use ui::ToastKind;

use super::ShellView;
use crate::file_prompt::FilePrompt;
use crate::file_reply::{self, Reply};
use crate::focused_chat::focused_chat;
use crate::notice_key::NoticeKey;
use crate::open_all::{self, OpenAll};
use crate::pane_spec::PaneSpec;
use crate::pending_file::PendingFile;
use crate::prompt_event::PromptEvent;

fn warn_unserved(command: &Command) {
    tracing::warn!(
        ?command,
        "the sidebar command needs daemon data the shell does not read yet"
    );
}

impl ShellView {
    pub(super) fn perform_file_action(
        &mut self,
        command: &Command,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match command {
            Command::OpenAllFiles { path } => self.list_to_open(path, window, cx),
            Command::OpenTerminalAt { path } => self.open_terminal_at(path, cx),
            Command::UploadFile { path } => Self::pick_uploads(path, window, cx),
            Command::UploadPaths { path, files } => {
                self.upload((path, files.clone()), (window, cx));
            },
            Command::AddFolderToWorkspace => self.choose_repo_folder(window, cx),
            Command::CreateEnvTemplate { path, target } => {
                self.make_env_template((path, target), Overwrite::Ask, (window, cx));
            },
            other => warn_unserved(other),
        }
    }

    fn fail(
        &mut self,
        reason: String,
        cx: &mut Context<'_, Self>,
    ) {
        self.show_notice(NoticeKey::Command, (ToastKind::Error, reason), cx);
    }

    fn when_answered<T: Send + 'static>(
        reply: Reply<T>,
        then: impl FnOnce(&mut Self, T, &mut Window, &mut Context<'_, Self>) + 'static,
        (window, cx): (&mut Window, &mut Context<'_, Self>),
    ) {
        cx.spawn_in(window, async move |this, cx| {
            deliver(reply, (Box::new(then), Self::fail), (this, cx)).await;
        })
        .detach();
    }

    fn list_to_open(
        &mut self,
        directory: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let (done, reply) = file_reply::channel();
        if let Err(error) = self.data.commands().list_files(directory, done) {
            return self.fail(error.to_string(), cx);
        }
        let then = |this: &mut Self, files, window: &mut Window, cx: &mut Context<'_, Self>| {
            this.plan_open(files, window, cx);
        };
        Self::when_answered(reply, then, (window, cx));
    }

    fn plan_open(
        &mut self,
        files: Vec<String>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match open_all::plan(files) {
            OpenAll::Nothing => {},
            OpenAll::Now(files) => self.open_each(&files, cx),
            OpenAll::Ask(files) => self.ask(PendingFile::OpenAll(files), window, cx),
        }
    }

    fn open_each(
        &mut self,
        files: &[String],
        cx: &mut Context<'_, Self>,
    ) {
        for file in files {
            self.open_workspace_file(file, cx);
        }
    }

    fn open_terminal_at(
        &mut self,
        directory: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let id = format!("t{}", self.state.serials().next_pane());
        let chat = focused_chat(self.state.layout());
        let folder = Some(directory).filter(|name| !name.is_empty());
        self.open_spec(PaneSpec::terminal(&id, chat.as_deref(), folder), cx);
    }

    fn pick_uploads(
        directory: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let picked = cx.prompt_for_paths(PathPromptOptions {
            files: true,
            directories: false,
            multiple: true,
            prompt: Some("Upload".into()),
        });
        let directory = directory.to_owned();
        cx.spawn_in(window, async move |this, cx| {
            let paths = picked.await.ok().and_then(Result::ok).flatten();
            let paths = paths.unwrap_or_default();
            this.update_in(cx, |view, w, c| view.upload((&directory, paths), (w, c)))
                .ok();
        })
        .detach();
    }

    fn upload(
        &mut self,
        (directory, paths): (&str, Vec<PathBuf>),
        (window, cx): (&mut Window, &mut Context<'_, Self>),
    ) {
        if paths.is_empty() {
            return;
        }
        let (done, reply) = file_reply::channel();
        if let Err(error) = self.data.commands().upload_files(directory, paths, done) {
            return self.fail(error.to_string(), cx);
        }
        let then = |_: &mut Self, _: Vec<String>, _: &mut Window, _: &mut Context<'_, Self>| {};
        Self::when_answered(reply, then, (window, cx));
    }

    fn make_env_template(
        &mut self,
        (source, target): (&str, &str),
        overwrite: Overwrite,
        (window, cx): (&mut Window, &mut Context<'_, Self>),
    ) {
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
                this.ask_if_exists(&outcome, pending, (window, cx));
            };
        Self::when_answered(reply, then, (window, cx));
    }

    fn ask_if_exists(
        &mut self,
        outcome: &TemplateOutcome,
        pending: PendingFile,
        (window, cx): (&mut Window, &mut Context<'_, Self>),
    ) {
        if matches!(outcome, TemplateOutcome::Exists(_)) {
            self.ask(pending, window, cx);
        }
    }

    pub(super) fn prompt_layer(&self) -> Option<gpui::Div> {
        let prompt = self.file_prompt.clone()?;
        Some(gpui::div().absolute().size_full().child(prompt))
    }

    fn ask(
        &mut self,
        pending: PendingFile,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.file_prompt.is_some() {
            return;
        }
        let prompt = cx.new(|_| FilePrompt::new(pending));
        self.subscriptions
            .push(cx.subscribe_in(&prompt, window, Self::on_file_prompt));
        self.file_prompt = Some(prompt);
        self.update_overlay(cx);
    }

    #[expect(
        clippy::too_many_arguments,
        reason = "gpui subscribe_in handlers receive self, entity, event, window and context"
    )]
    fn on_file_prompt(
        &mut self,
        _: &Entity<FilePrompt>,
        event: &PromptEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.file_prompt = None;
        self.update_overlay(cx);
        let PromptEvent::Confirmed(pending) = event else {
            return;
        };
        match pending {
            PendingFile::OpenAll(files) => self.open_each(files, cx),
            PendingFile::Overwrite { path, target } => {
                self.make_env_template((path, target), Overwrite::Replace, (window, cx));
            },
        }
    }
}

type Then<V, T> = Box<dyn FnOnce(&mut V, T, &mut Window, &mut Context<'_, V>)>;
type Failed<V> = fn(&mut V, String, &mut Context<'_, V>);

async fn deliver<V: 'static, T: Send + 'static>(
    reply: Reply<T>,
    (then, failed): (Then<V, T>, Failed<V>),
    (this, cx): (WeakEntity<V>, &mut AsyncWindowContext),
) {
    let Ok(answer) = reply.recv().await else {
        return;
    };
    this.update_in(cx, |this, window, cx| match answer {
        Ok(value) => then(this, value, window, cx),
        Err(reason) => failed(this, reason.to_string(), cx),
    })
    .ok();
}

#[cfg(test)]
mod tests {
    use gpui::{IntoElement, Render, TestAppContext};

    use super::*;

    #[derive(Default)]
    struct Probe {
        heard: Vec<String>,
    }

    fn fail(
        probe: &mut Probe,
        reason: String,
        _: &mut Context<'_, Probe>,
    ) {
        probe.heard.push(reason);
    }

    impl Render for Probe {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            gpui::div()
        }
    }

    fn start(
        reply: Reply<u8>,
        then: Then<Probe, u8>,
        (window, cx): (&mut Window, &mut Context<'_, Probe>),
    ) {
        cx.spawn_in(window, async move |this, cx| {
            deliver(reply, (then, fail), (this, cx)).await;
        })
        .detach();
    }

    fn heard_after(
        cx: &mut TestAppContext,
        answer: Result<u8, shell_data::FileError>,
    ) -> Vec<String> {
        let (probe, cx) = cx.add_window_view(|_, _| Probe::default());
        let (done, reply) = file_reply::channel();
        done(answer);
        let then: Then<Probe, u8> = Box::new(|probe, value, _, _| {
            probe.heard.push(format!("got {value}"));
        });
        probe.update_in(cx, |_, window, cx| start(reply, then, (window, cx)));
        cx.run_until_parked();
        probe.read_with(cx, |probe, _| probe.heard.clone())
    }

    #[gpui::test]
    fn an_answer_reaches_the_view_and_a_failure_reaches_its_handler(cx: &mut TestAppContext) {
        assert_eq!(heard_after(cx, Ok(7)), ["got 7"]);
        let refused = shell_data::FileError::Local {
            path: "disk".into(),
            source: std::io::Error::other("no space"),
        };
        assert_eq!(heard_after(cx, Err(refused)), ["disk: no space"]);
    }
}
