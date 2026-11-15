use std::path::PathBuf;

use gpui::{AsyncApp, Context, PathPromptOptions, WeakEntity, Window};
use shell_data::{Outcome as DataOutcome, SettingsAdapter};
use std::sync::atomic::Ordering;
use ui::ToastKind;

use super::ShellView;
use crate::core_panes::{chat_params, chat_type};
use crate::notice_key::NoticeKey;
use crate::settings_modal::open_settings;

type Picked<E, C> = Result<Result<Option<Vec<PathBuf>>, E>, C>;

fn first_read<E, C>(picked: Picked<E, C>) -> Option<std::io::Result<Vec<u8>>> {
    let paths = picked.ok()?.ok()??;
    paths.first().map(std::fs::read)
}

fn deliver(
    this: &WeakEntity<ShellView>,
    cx: &mut AsyncApp,
    read: Option<std::io::Result<Vec<u8>>>,
) {
    let Some(read) = read else {
        return;
    };
    this.update(cx, |this, cx| this.install_descriptor(read, cx))
        .ok();
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
        if let Some(dialog) = &self.branch_import {
            dialog.update(cx, |_, cx| cx.notify());
        }
        if let Some(dialog) = &self.remote_folders {
            dialog.update(cx, |_, cx| cx.notify());
        }
        if let Some(dialog) = &self.pairing {
            dialog.update(cx, pairing_view::PairingView::changed);
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
            DataOutcome::PickDescriptor => Self::pick_descriptor(cx),
            DataOutcome::PairHost => self.show_pairing(window, cx),
            DataOutcome::CopyText(text) => {
                cx.write_to_clipboard(gpui::ClipboardItem::new_string(text));
            },
            DataOutcome::Failed(reason) => {
                tracing::warn!(%reason, "a command failed");
                self.show_notice(NoticeKey::Command, (ToastKind::Error, reason), cx);
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
        self.state.open_pane(type_id, params);
        self.refresh(cx);
    }

    fn pick_descriptor(cx: &mut Context<'_, Self>) {
        let picked = cx.prompt_for_paths(PathPromptOptions {
            files: true,
            directories: false,
            multiple: false,
            prompt: Some("Install".into()),
        });
        cx.spawn(async move |this, cx| deliver(&this, cx, first_read(picked.await)))
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
            .map(SettingsAdapter::new)
    }

    pub(super) fn show_settings(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let opened = open_settings(self.requests.clone(), &backend, window, cx);
        self.settings = opened
            .inspect_err(|error| tracing::error!(%error, "could not open settings"))
            .ok();
        self.update_overlay(cx);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_first_picked_file_is_read_and_a_cancelled_pick_reads_nothing() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("agent.yaml");
        std::fs::write(&file, "name: a").unwrap();
        let picked = first_read::<(), ()>(Ok(Ok(Some(vec![file]))));
        assert_eq!(picked.unwrap().unwrap(), b"name: a", "read");
        assert!(first_read::<(), ()>(Ok(Ok(None))).is_none(), "cancelled");
        assert!(
            first_read::<(), ()>(Ok(Ok(Some(Vec::new())))).is_none(),
            "empty"
        );
        assert!(first_read::<(), ()>(Err(())).is_none(), "prompt dropped");
    }
}
