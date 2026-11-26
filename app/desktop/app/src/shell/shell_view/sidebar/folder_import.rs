use std::path::PathBuf;
use std::rc::Rc;

use folderimport::{ImportEvent, Kind};
use folderpicker::PickerEvent;
use gpui::{AppContext as _, Context, Entity, Window};
use sidebar::{Command, Requests as _, Sink as _};
use store::FolderChoice;

use crate::shell::inbox::data_signal;
use crate::shell::shell_view::ShellView;
use crate::shell::shell_view::events::first_run::landing;
use crate::shell::window::notice_key::NoticeKey;

// SPEC: docs/plans/2026-11-26-overlay-rule.md#the-rule rfd's GTK folder chooser.
pub(crate) const IMPORT_FOLDER: ui::FileChoice = ui::FileChoice {
    title: "Select Folder",
    kind: ui::ChoiceKind::Folder,
    filter: None,
};

impl ShellView {
    pub(crate) fn show_folder_import(
        &mut self,
        kind: Kind,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.folder_import.is_some() {
            return;
        }
        let view = self.data.view();
        let adding: folderimport::Adding = Rc::new(move |folder| view.adding(folder));
        let dialog = cx.new(|cx| folderimport::View::new((kind, adding), window, cx));
        let subscription = cx.subscribe_in(&dialog, window, move |this, _, event, window, cx| {
            this.on_folder_import((kind, event), window, cx);
        });
        self.subscriptions.push(subscription);
        self.folder_import = Some(dialog);
        cx.notify();
    }

    fn on_folder_import(
        &mut self,
        (kind, event): (Kind, &ImportEvent),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match event {
            ImportEvent::Close => {
                self.folder_import = None;
                cx.notify();
            },
            ImportEvent::Browse => self.browse_import_folder(kind, window, cx),
            ImportEvent::Add { folder, name } => self.add_folder(kind, (folder, name), cx),
        }
    }

    fn browse_import_folder(
        &mut self,
        kind: Kind,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if kind == Kind::Repo && self.data.folder_choice() == FolderChoice::OnHost {
            self.show_remote_folders(cx);
            return;
        }
        let picked = ui::FileChooser::choose(IMPORT_FOLDER, window, cx);
        cx.spawn_in(window, async move |this, cx| {
            let path = first_path(&picked.await)?;
            this.update(cx, |this, cx| this.import_folder_chosen(&path, cx))
                .ok()
        })
        .detach();
    }

    pub(crate) fn import_folder_chosen(
        &mut self,
        path: &str,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(dialog) = &self.folder_import {
            dialog.update(cx, |dialog, cx| dialog.set_folder(path, cx));
        }
    }

    fn add_folder(
        &mut self,
        kind: Kind,
        (folder, name): (&str, &str),
        cx: &mut Context<'_, Self>,
    ) {
        let sent = match kind {
            Kind::Repo => self.data.commands().import_repo((folder, name)),
            Kind::Space => self.create_local_space((folder, name)),
        };
        if let Err(error) = sent {
            tracing::warn!(%error, "the folder was not added");
            self.show_notice(
                NoticeKey::Command,
                (ui::ToastKind::Error, error.to_string()),
                cx,
            );
        }
    }

    fn create_local_space(
        &self,
        (folder, name): (&str, &str),
    ) -> Result<(), sidebar::Error> {
        let Some(host) = self.local_host_id() else {
            return Ok(());
        };
        let name = Some(name.to_owned())
            .filter(|name| !name.is_empty())
            .or_else(|| landing::space_name(folder))
            .unwrap_or_else(|| folder.to_owned());
        let command = Command::CreateSpace {
            name,
            folder: folder.to_owned(),
            host,
        };
        self.data.commands().dispatch(command)
    }

    pub(crate) fn folder_added(
        &mut self,
        folder: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.sidebar
            .update(cx, |sidebar, cx| sidebar.space_added(folder, cx));
        if let Some(dialog) = &self.folder_import {
            dialog.update(cx, |dialog, cx| dialog.added(folder, cx));
        }
    }

    pub(crate) fn show_remote_folders(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.remote_folders.is_some() {
            return;
        }
        let (signal, _) = data_signal::callbacks(&self.requests, &self.data_queued);
        let opened = self
            .data
            .folder_browse(signal)
            .inspect_err(|error| tracing::warn!(%error, "the folder picker could not open"));
        let Ok(browse) = opened else {
            return;
        };
        let picker = cx.new(|_| folderpicker::View::new(browse));
        self.subscriptions
            .push(cx.subscribe(&picker, Self::on_remote_folder));
        self.remote_folders = Some(picker);
        cx.notify();
    }

    fn on_remote_folder(
        &mut self,
        _: Entity<folderpicker::View>,
        event: &PickerEvent,
        cx: &mut Context<'_, Self>,
    ) {
        self.remote_folders = None;
        cx.notify();
        if let PickerEvent::Chosen(path) = event {
            self.import_folder_chosen(path, cx);
        }
    }
}

fn first_path(paths: &[PathBuf]) -> Option<String> {
    paths
        .first()
        .map(|path| path.to_string_lossy().into_owned())
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::shell::shell_view::tests::open_shell;
    use crate::test_support::answer_choices_with;

    fn dialog_open(opened: &mut crate::shell::shell_view::tests::Opened) -> bool {
        opened.window.run_until_parked();
        opened
            .shell
            .update(&mut opened.window, |shell, _| shell.folder_import.is_some())
    }

    #[gpui::test]
    fn import_a_repo_opens_develop_s_add_repository_dialog_not_the_chooser(
        cx: &mut TestAppContext
    ) {
        let asked = answer_choices_with(cx, &["/srv/app"]);
        let mut opened = open_shell(cx);
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.on_sidebar_command(&Command::ImportRepo, window, cx);
            });
        assert!(
            dialog_open(&mut opened),
            "Import a repo asks in a dialog first"
        );
        assert!(asked.borrow().is_empty(), "no chooser before Browse");
        let dialog = opened
            .shell
            .update(&mut opened.window, |shell, _| shell.folder_import.clone());
        let dialog = dialog.unwrap();
        opened.window.update(|_, cx| {
            dialog.update(cx, |_, cx| cx.emit(ImportEvent::Browse));
        });
        opened.window.run_until_parked();
        assert_eq!(*asked.borrow(), [IMPORT_FOLDER]);
    }

    #[gpui::test]
    fn the_added_folder_closes_the_dialog_that_asked_for_it(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        opened
            .shell
            .update_in(&mut opened.window, |shell, window, cx| {
                shell.show_folder_import(Kind::Space, window, cx);
                shell.import_folder_chosen("/srv/app", cx);
                shell.folder_added("/srv/app", cx);
            });
        assert!(
            !dialog_open(&mut opened),
            "the space exists, the dialog closes"
        );
    }

    type FirstPath = (&'static [&'static str], Option<&'static str>);

    const FIRST_PATHS: [FirstPath; 2] = [(&["/a/one", "/a/two"], Some("/a/one")), (&[], None)];

    #[test]
    fn the_first_picked_path_is_the_one_kept_and_an_empty_pick_keeps_nothing() {
        for (picked, first) in FIRST_PATHS {
            let picked: Vec<PathBuf> = picked.iter().map(PathBuf::from).collect();
            assert_eq!(first_path(&picked).as_deref(), first, "{picked:?}");
        }
    }
}
