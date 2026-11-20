use std::path::PathBuf;
use std::sync::Arc;

use gpui::{AsyncApp, Context, PathPromptOptions, WeakEntity, Window};
use std::sync::atomic::Ordering;
use store::{Outcome as DataOutcome, SettingsAdapter};
use ui::ToastKind;

use super::ShellView;
use crate::byte_source::ByteSource;
use crate::core_panes::{chat_params, chat_type};
use crate::logged::{closed_view, logged};
use crate::notice_key::NoticeKey;
use crate::settings_modal::{open_settings, settings_config};

type Picked<E, C> = Result<Result<Option<Vec<PathBuf>>, E>, C>;

fn first_read<E, C>(
    picked: Picked<E, C>,
    files: &dyn ByteSource,
) -> Option<std::io::Result<Vec<u8>>> {
    let paths = picked.ok()?.ok()??;
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
    pub(super) fn on_data_changed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.data_queued.store(false, Ordering::SeqCst);
        if let Some(backend) = &self.settings_backend {
            backend.deliver_pending();
        }
        self.sidebar.update(cx, |_, cx| cx.notify());
        self.update_landing(cx);
        self.push_tabs(cx);
        if self.source.native().terminals_attached() {
            self.refresh(cx);
        }
        if let Some(dialog) = &self.branch_import {
            dialog.update(cx, |_, cx| cx.notify());
        }
        if let Some(dialog) = &self.remote_folders {
            dialog.update(cx, |_, cx| cx.notify());
        }
        if let Some(dialog) = &self.pairing {
            dialog.update(cx, pairing::View::changed);
        }
    }

    pub(super) fn on_data_outcome(
        &mut self,
        outcome: DataOutcome,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match outcome {
            DataOutcome::OpenChat(chat) => self.open_chat(&chat, cx),
            DataOutcome::PickDescriptor => self.pick_descriptor(cx),
            DataOutcome::PairHost => self.show_pairing(window, cx),
            DataOutcome::ClearBrowsingData => self.clear_browsing_data(cx),
            DataOutcome::CopyText(text) => {
                cx.write_to_clipboard(gpui::ClipboardItem::new_string(text));
            },
            DataOutcome::NameRow { row, name } => self.sidebar.update(cx, |sidebar, cx| {
                sidebar.name_new_row((row, &name), window, cx);
            }),
            DataOutcome::Failed(reason) => {
                tracing::warn!(%reason, "a command failed");
                self.show_notice(NoticeKey::Command, (ToastKind::Error, reason), cx);
            },
            DataOutcome::WorkAtRisk { row, at_risk } => {
                self.sidebar
                    .update(cx, |sidebar, cx| sidebar.ask_to_discard(&row, at_risk, cx));
            },
            DataOutcome::RemovalRefused { row, reason } => {
                tracing::warn!(%reason, "a delete was refused");
                self.sidebar
                    .update(cx, |sidebar, cx| sidebar.removal_refused(&row, reason, cx));
            },
        }
    }

    fn open_chat(
        &mut self,
        chat: &str,
        cx: &mut Context<'_, Self>,
    ) {
        let (Ok(type_id), Ok(params)) = (chat_type(), chat_params(chat)) else {
            return;
        };
        self.open_from_sidebar((type_id, params), cx);
    }

    fn pick_descriptor(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        let picked = cx.prompt_for_paths(PathPromptOptions {
            files: true,
            directories: false,
            multiple: false,
            prompt: Some("Install".into()),
        });
        let files = Arc::clone(&self.ports.bytes);
        cx.spawn(async move |this, cx| {
            let read = first_read(picked.await, files.as_ref());
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

    pub(super) fn settings_backend(&mut self) -> Option<SettingsAdapter> {
        if self.settings_backend.is_none() {
            self.settings_backend = self.open_settings_backend();
        }
        self.settings_backend.clone()
    }

    fn open_settings_backend(&self) -> Option<SettingsAdapter> {
        let (signal, _) = crate::data_signal::callbacks(&self.requests, &self.data_queued);
        let file = self.preferences_path.with_file_name("settings.json");
        self.data
            .settings(file, signal, self.system_fonts.clone())
            .inspect_err(|error| tracing::error!(%error, "the settings could not be opened"))
            .ok()
    }

    pub(super) fn show_settings(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.show_settings_page(None, window, cx);
    }

    pub(super) fn show_settings_page(
        &mut self,
        page: Option<settings::PageId>,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let opened = settings_config(self.requests.clone(), &backend, page)
            .map(|config| open_settings(config, window, cx));
        self.settings = logged(opened, "could not open settings");
        if let Some(modal) = &self.settings {
            modal.read(cx).focus(window, cx);
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
        let read = first_read::<(), ()>(Ok(Ok(Some(chosen(vec![DESCRIPTOR])))), &files);
        assert_eq!(read.unwrap().unwrap(), b"name: a", "read");
        assert!(
            first_read::<(), ()>(Ok(Ok(None)), &files).is_none(),
            "cancelled"
        );
        assert!(
            first_read::<(), ()>(Ok(Ok(Some(chosen(Vec::new())))), &files).is_none(),
            "empty"
        );
        assert!(
            first_read::<(), ()>(Err(()), &files).is_none(),
            "prompt dropped"
        );
        let missing = first_read::<(), ()>(Ok(Ok(Some(chosen(vec!["/picked/none.yaml"])))), &files);
        assert!(missing.unwrap().is_err(), "unreadable");
    }
}
