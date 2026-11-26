use std::path::Path;

use branchimport::ImportEvent;
use diagnostics::LogPaths;
use gpui::{AppContext as _, Context, Entity, Window};
use settings::RecordBackend as _;

use super::ShellView;
use crate::daemon::diagnostics_export;
use crate::shell::inbox::data_signal;

impl ShellView {
    pub(in crate::shell::shell_view) fn show_pairing(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.pairing.is_some() {
            return;
        }
        let (signal, _) = data_signal::callbacks(&self.requests, &self.data_queued);
        let session = self.data.pairing(signal);
        let dialog = cx.new(|cx| pairing::View::new(session, window, cx));
        self.subscriptions.push(cx.subscribe(
            &dialog,
            |this, source, event: &pairing::Event, cx| {
                this.on_pairing(source, *event, cx);
            },
        ));
        self.pairing = Some(dialog);
        cx.notify();
    }

    fn on_pairing(
        &mut self,
        _: Entity<pairing::View>,
        event: pairing::Event,
        cx: &mut Context<'_, Self>,
    ) {
        if matches!(event, pairing::Event::Close) {
            self.pairing = None;
            cx.notify();
        }
    }

    pub(in crate::shell::shell_view) fn show_device_offer(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.device_offer.is_some() {
            return;
        }
        let (signal, _) = data_signal::callbacks(&self.requests, &self.data_queued);
        let offer = match self.data.device_offer(signal) {
            Ok(offer) => offer,
            Err(error) => {
                return tracing::warn!(%error, "no device can pair without a local daemon");
            },
        };
        let dialog = cx.new(|_| pairing::OfferView::new(offer));
        self.subscriptions
            .push(cx.subscribe(&dialog, |this, _, _: &pairing::Event, cx| {
                this.close_device_offer(cx);
            }));
        self.device_offer = Some(dialog);
        cx.notify();
    }

    // SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/hosts/components/add-device-dialog.tsx
    fn close_device_offer(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.device_offer = None;
        cx.notify();
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let reread = backend.apply(
            settings::DEVICE_RECORDS,
            "",
            settings::RecordCommand::Refresh,
        );
        if let Err(error) = reread {
            tracing::warn!(%error, "the paired devices were not read again");
        }
    }

    pub(in crate::shell::shell_view) fn show_branch_import(
        &mut self,
        row: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.branch_import.is_some() {
            return;
        }
        let (signal, _) = data_signal::callbacks(&self.requests, &self.data_queued);
        let opened = self
            .data
            .branch_import(&sidebar::RowId::from(row), signal)
            .inspect_err(|error| tracing::warn!(%error, "the branch import could not open"));
        let Ok(import) = opened else {
            return;
        };
        let dialog = cx.new(|cx| branchimport::View::new(import, window, cx));
        self.subscriptions.push(
            cx.subscribe(&dialog, |this, source, event: &ImportEvent, cx| {
                this.on_branch_import(source, *event, cx);
            }),
        );
        self.branch_import = Some(dialog);
        cx.notify();
    }

    fn on_branch_import(
        &mut self,
        _: Entity<branchimport::View>,
        _: ImportEvent,
        cx: &mut Context<'_, Self>,
    ) {
        self.branch_import = None;
        cx.notify();
    }

    pub(in crate::shell::shell_view) fn export_diagnostics(&self) {
        let data_dir = data_dir_of(&self.preferences_path);
        diagnostics_export::start(
            &self.link,
            LogPaths::in_data_dir(data_dir),
            self.ports.clock.wall(),
        );
    }

    pub(in crate::shell::shell_view) fn open_log(&self) {
        let data_dir = data_dir_of(&self.preferences_path);
        let log = LogPaths::in_data_dir(data_dir).current();
        if let Err(error) = platform::open_path(&log) {
            tracing::warn!(%error, "the log file could not be opened");
        }
    }
}

fn data_dir_of(preferences_path: &Path) -> &Path {
    preferences_path.parent().unwrap_or(Path::new(""))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_data_directory_is_where_the_preferences_live() {
        let cases = [("/data/sidebar.json", "/data"), ("sidebar.json", "")];
        for (file, directory) in cases {
            assert_eq!(data_dir_of(Path::new(file)), Path::new(directory), "{file}");
        }
    }
}
