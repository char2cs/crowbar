use std::path::PathBuf;

use gpui::{Context, Window};
use sidebar::Command;
use ui::ToastKind;

use super::ShellView;
use crate::file_reply::{self, Reply};
use crate::logged::closed_view;
use crate::notice_key::NoticeKey;
use crate::open_all::{self, OpenAll};
use crate::panes::kinds::pane_spec::PaneSpec;
use crate::panes::tracking::focused_chat::focused_chat;
use crate::pending_file::PendingFile;

mod answers;
mod handlers;
mod template_prompt;
mod template_request;

use self::answers::deliver;
use self::handlers::Handlers;
use self::template_request::TemplateRequest;

// SPEC: docs/triage/2026-11-18-side-by-side-audit/16-dlg.md DLG-049: WebKit's chooser for an `<input type=file multiple>`.
const UPLOADS: ui::FileChoice = ui::FileChoice {
    title: "Select Files",
    kind: ui::ChoiceKind::Files,
    filter: None,
};

fn warn_unserved(command: &Command) {
    tracing::warn!(
        ?command,
        "the sidebar command needs daemon data the shell does not read yet"
    );
}

fn terminal_folder(directory: &str) -> Option<&str> {
    Some(directory).filter(|name| !name.is_empty())
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
            Command::CreateEnvTemplate { path, target } => {
                let request = TemplateRequest::asking(path, target);
                self.make_env_template(&request, window, cx);
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
        let handlers = Handlers {
            then: Box::new(then),
            failed: Self::fail,
        };
        cx.spawn_in(window, async move |this, cx| {
            deliver(reply, handlers, this, cx).await;
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

    pub(super) fn open_terminal_at(
        &mut self,
        directory: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let id = format!("t{}", self.state.serials().next_pane());
        let chat = focused_chat(self.state.layout(), &self.state.space());
        let folder = terminal_folder(directory);
        self.open_spec(PaneSpec::terminal(&id, chat.as_deref(), folder), cx);
    }

    fn pick_uploads(
        directory: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let picked = ui::FileChooser::choose(UPLOADS, window, cx);
        let directory = directory.to_owned();
        cx.spawn_in(window, async move |this, cx| {
            let paths = picked.await;
            let uploaded = cx
                .update(|w, c| this.update(c, |view, c| view.upload((&directory, paths), (w, c))))
                .flatten();
            closed_view(&uploaded, "the upload picker's answer");
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
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::shell_view::tests::open_shell;
    use crate::test_support::answer_choices_with;

    #[gpui::test]
    fn upload_file_asks_develop_s_multiple_file_chooser(cx: &mut TestAppContext) {
        let asked = answer_choices_with(cx, &[]);
        let mut opened = open_shell(cx);
        let shell = opened.shell.clone();
        shell.update_in(&mut opened.window, |_, window, cx| {
            ShellView::pick_uploads("src", window, cx);
        });
        opened.window.run_until_parked();
        assert_eq!(*asked.borrow(), [UPLOADS]);
    }

    #[test]
    fn a_terminal_opens_in_the_given_folder_or_the_default_for_the_workspace_root() {
        let cases = [("src", Some("src")), ("", None)];
        for (directory, expected) in cases {
            assert_eq!(terminal_folder(directory), expected, "{directory:?}");
        }
    }
}
