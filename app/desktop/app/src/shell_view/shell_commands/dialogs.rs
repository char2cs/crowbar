use std::path::Path;

use branch_import_view::{BranchImportView, ImportEvent};
use gpui::{AppContext as _, Context, Entity, Window};
use pairing_view::{PairingEvent, PairingView};
use shell_diagnostics::LogPaths;

use super::ShellView;

impl ShellView {
    pub(in crate::shell_view) fn show_pairing(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.pairing.is_some() {
            return;
        }
        let (signal, _) = crate::data_signal::callbacks(&self.requests, &self.data_queued);
        let session = self.data.pairing(signal);
        let dialog = cx.new(|cx| PairingView::new(session, window, cx));
        self.subscriptions.push(
            cx.subscribe(&dialog, |this, source, event: &PairingEvent, cx| {
                this.on_pairing(source, *event, cx);
            }),
        );
        self.pairing = Some(dialog);
        self.update_overlay(cx);
    }

    fn on_pairing(
        &mut self,
        _: Entity<PairingView>,
        event: PairingEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if matches!(event, PairingEvent::Close) {
            self.pairing = None;
            self.update_overlay(cx);
        }
    }

    pub(in crate::shell_view) fn show_branch_import(
        &mut self,
        row: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.branch_import.is_some() {
            return;
        }
        let (signal, _) = crate::data_signal::callbacks(&self.requests, &self.data_queued);
        let opened = self
            .data
            .branch_import(&sidebar::RowId::from(row), signal)
            .inspect_err(|error| tracing::warn!(%error, "the branch import could not open"));
        let Ok(import) = opened else {
            return;
        };
        let dialog = cx.new(|cx| BranchImportView::new(import, window, cx));
        self.subscriptions.push(
            cx.subscribe(&dialog, |this, source, event: &ImportEvent, cx| {
                this.on_branch_import(source, *event, cx);
            }),
        );
        self.branch_import = Some(dialog);
        self.update_overlay(cx);
    }

    fn on_branch_import(
        &mut self,
        _: Entity<BranchImportView>,
        _: ImportEvent,
        cx: &mut Context<'_, Self>,
    ) {
        self.branch_import = None;
        self.update_overlay(cx);
    }

    pub(in crate::shell_view) fn export_diagnostics(&self) {
        let data_dir = data_dir_of(&self.preferences_path);
        crate::diagnostics_export::start(
            &self.link,
            LogPaths::in_data_dir(data_dir),
            self.ports.clock.wall(),
        );
    }

    pub(in crate::shell_view) fn open_log(&self) {
        let data_dir = data_dir_of(&self.preferences_path);
        let log = LogPaths::in_data_dir(data_dir).current();
        if let Err(error) = shell_platform::open_path(&log) {
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
