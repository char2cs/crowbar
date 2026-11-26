use std::path::PathBuf;
use std::sync::Arc;

use gpui::{AsyncApp, ClipboardItem, Context, WeakEntity, Window};
use std::sync::atomic::Ordering;
use store::{Outcome as DataOutcome, SettingsAdapter};
use ui::ToastKind;

use crate::panes::kinds::core_panes::{chat_params, chat_type, draft_params};
use crate::shell::inbox::data_signal;
use crate::shell::inbox::logged::{closed_view, logged};
use crate::shell::ports::byte_source::ByteSource;
use crate::shell::shell_view::ShellView;
use crate::shell::shell_view::settings::settings_modal::{open_settings, settings_config};
use crate::shell::window::notice_key::NoticeKey;

// SPEC: docs/triage/2026-11-18-side-by-side-audit/15-set.md SET-094: WebKit's chooser for the descriptor `<input accept=…>`.
const DESCRIPTOR_CHOICE: ui::FileChoice = ui::FileChoice {
    title: "Select File",
    kind: ui::ChoiceKind::File,
    filter: Some(ui::FileFilter {
        name: None,
        extensions: &["yaml", "yml"],
        mime_types: &["application/yaml", "text/yaml"],
    }),
};

fn first_read(
    paths: &[PathBuf],
    files: &dyn ByteSource,
) -> Option<std::io::Result<Vec<u8>>> {
    paths.first().map(|path| files.read_bytes(path))
}

fn deliver(
    this: &WeakEntity<ShellView>,
    cx: &mut AsyncApp,
    read: Option<std::io::Result<Vec<u8>>>,
) {
    let Some(read) = read else {
        return;
    };
    let delivered = this.update(cx, |this, cx| this.install_descriptor(read, cx));
    closed_view(&delivered, "the descriptor");
}

impl ShellView {
    pub(crate) fn on_data_changed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.data_queued.store(false, Ordering::SeqCst);
        if let Some(backend) = &self.settings_backend {
            backend.deliver_pending();
        }
        self.sidebar.update(cx, |_, cx| cx.notify());
        self.backend.workspace.update(cx, |_, cx| cx.notify());
        self.state.set_paired_hosts(&self.data.paired_hosts());
        let departed = self.data.take_departed_chats();
        self.close_chats(&departed, cx);
        self.source.native().editors().data_changed(cx);
        self.update_landing(cx);
        self.follow_backend(cx);
        let left = self.data.take_departed_projects();
        let closing = self.state.closing_spaces(&left);
        if !closing.is_empty() {
            self.apply_guarded(closing, cx);
        }
        self.state.adopt_unowned();
        self.show_active_space(cx);
        if let Some(dialog) = &self.branch_import {
            dialog.update(cx, |_, cx| cx.notify());
        }
        if let Some(dialog) = &self.remote_folders {
            dialog.update(cx, |_, cx| cx.notify());
        }
        if let Some(dialog) = &self.folder_import {
            dialog.update(cx, |_, cx| cx.notify());
        }
        if let Some(dialog) = &self.pairing {
            dialog.update(cx, pairing::View::changed);
        }
        if let Some(dialog) = &self.device_offer {
            dialog.update(cx, |_, cx| cx.notify());
        }
    }

    pub(crate) fn on_data_outcome(
        &mut self,
        outcome: DataOutcome,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match outcome {
            DataOutcome::OpenChat(chat) => self.open_chat(chat_params(&chat), cx),
            DataOutcome::OpenDraft(draft) => self.open_chat(draft_params(&draft), cx),
            DataOutcome::PickDescriptor => self.pick_descriptor(window, cx),
            DataOutcome::PairHost => self.show_pairing(window, cx),
            DataOutcome::AddDevice => self.show_device_offer(cx),
            DataOutcome::ClearBrowsingData => self.clear_browsing_data(cx),
            DataOutcome::CopyText(text) => cx.write_to_clipboard(ClipboardItem::new_string(text)),
            DataOutcome::NameRow { row, name } => self.sidebar.update(cx, |sidebar, cx| {
                sidebar.name_new_row((row, &name), window, cx);
            }),
            DataOutcome::NameAnswered { row, refusal } => self
                .sidebar
                .update(cx, |sidebar, cx| sidebar.name_answered(&row, refusal, cx)),
            DataOutcome::Failed(reason) => {
                tracing::warn!(%reason, "a command failed");
                self.show_notice(NoticeKey::Command, (ToastKind::Error, reason), cx);
            },
            DataOutcome::Refused { title, reason } => self.show_refusal(title, reason, cx),
            DataOutcome::ReviewViewed { workspace, viewed } => {
                self.push_review_viewed(&workspace, &viewed);
            },
            disk @ (DataOutcome::FileChanged { .. } | DataOutcome::FilesSynced { .. }) => {
                self.disk_changed(disk, cx);
            },
            DataOutcome::WorkAtRisk { row, at_risk } => self.sidebar.update(cx, |sidebar, cx| {
                sidebar.ask_to_discard((&row, at_risk), window, cx);
            }),
            DataOutcome::RemovalRefused { row, reason } => self
                .sidebar
                .update(cx, |sidebar, cx| sidebar.removal_refused(&row, reason, cx)),
            DataOutcome::AlsoRemoved { titles } => self
                .sidebar
                .update(cx, |sidebar, cx| sidebar.also_removed(&titles, cx)),
            DataOutcome::Added(folder) => self.folder_added(&folder, cx),
        }
    }

    pub(crate) fn open_chat(
        &mut self,
        params: crate::error::Result<protocol::Params>,
        cx: &mut Context<'_, Self>,
    ) {
        let (Ok(type_id), Ok(params)) = (chat_type(), params) else {
            return;
        };
        self.state.open_view(type_id, params);
        self.refresh(cx);
    }

    fn pick_descriptor(
        &self,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) {
        let picked = ui::FileChooser::choose(DESCRIPTOR_CHOICE, window, cx);
        let files = Arc::clone(&self.ports.bytes);
        cx.spawn(async move |this, cx| {
            let read = first_read(&picked.await, files.as_ref());
            deliver(&this, cx, read);
        })
        .detach();
    }

    fn install_descriptor(
        &mut self,
        read: std::io::Result<Vec<u8>>,
        cx: &mut Context<'_, Self>,
    ) {
        match (read, self.settings_backend()) {
            (Ok(text), Some(backend)) => backend.upload_descriptor(text),
            (Err(error), _) => {
                let notice = (
                    ToastKind::Error,
                    format!("The descriptor could not be read: {error}"),
                );
                self.show_notice(NoticeKey::Command, notice, cx);
            },
            (Ok(_), None) => tracing::warn!("the settings are not open; the descriptor is dropped"),
        }
    }

    pub(crate) fn settings_backend(&mut self) -> Option<SettingsAdapter> {
        if self.settings_backend.is_none() {
            self.settings_backend = self.open_settings_backend();
        }
        self.settings_backend.clone()
    }

    pub(crate) fn flush_settings_on_quit(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let quit = cx.on_app_quit(|this, _| {
            this.settings_backend
                .iter()
                .for_each(SettingsAdapter::flush);
            std::future::ready(())
        });
        self.subscriptions.push(quit);
    }

    fn open_settings_backend(&self) -> Option<SettingsAdapter> {
        let (signal, _) = data_signal::callbacks(&self.requests, &self.data_queued);
        let file = self.preferences_path.with_file_name("settings.json");
        self.data
            .settings(file, signal, self.system_fonts.clone())
            .inspect_err(|error| tracing::error!(%error, "the settings could not be opened"))
            .ok()
    }

    pub(crate) fn show_settings(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.show_settings_page(None, window, cx);
    }

    pub(crate) fn show_settings_page(
        &mut self,
        page: Option<settings::PageId>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let back = window.focused(cx);
        let opened = settings_config(self.requests.clone(), &backend, (page, back))
            .map(|config| open_settings(config, window, cx));
        self.settings = logged(opened, "could not open settings");
        if let Some(modal) = &self.settings {
            modal.update(cx, |modal, cx| modal.focus(window, cx));
        }
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use super::*;
    use crate::test_support::MemoryFiles;

    const DESCRIPTOR: &str = "/picked/agent.yaml";

    fn chosen(paths: Vec<&str>) -> Vec<PathBuf> {
        paths.into_iter().map(PathBuf::from).collect()
    }

    #[test]
    fn the_first_picked_file_is_read_and_a_cancelled_pick_reads_nothing() {
        let files = MemoryFiles::default();
        files.seed(Path::new(DESCRIPTOR), "name: a");
        let read = first_read(&chosen(vec![DESCRIPTOR]), &files);
        assert_eq!(read.unwrap().unwrap(), b"name: a", "read");
        assert!(first_read(&[], &files).is_none(), "cancelled");
        let missing = first_read(&chosen(vec!["/picked/none.yaml"]), &files);
        assert!(missing.unwrap().is_err(), "unreadable");
    }
}
