use gpui::{AppContext as _, Context, Entity, Window};
use remote_folder_picker::{PickerEvent, RemoteFolderPicker};
use shell_data::FolderChoice;

use super::ShellView;

fn on_host(choice: FolderChoice) -> bool {
    matches!(choice, FolderChoice::OnHost)
}

impl ShellView {
    pub(super) fn choose_repo_folder(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if on_host(self.data.folder_choice()) {
            self.show_remote_folders(cx);
            return;
        }
        Self::import_repo(window, cx);
    }

    pub(super) fn choose_space_folder(
        &mut self,
        host: &str,
        cx: &mut Context<'_, Self>,
    ) {
        if self.remote_folders.is_some() {
            return;
        }
        let (signal, _) = crate::data_signal::callbacks(&self.requests, &self.data_queued);
        let opened = self
            .data
            .folder_browse_on(host, signal)
            .inspect_err(|error| tracing::warn!(%error, "the folder picker could not open"));
        let Ok(browse) = opened else {
            return;
        };
        let picker = cx.new(|_| RemoteFolderPicker::new(browse));
        self.subscriptions
            .push(cx.subscribe(&picker, Self::on_space_folder));
        self.remote_folders = Some(picker);
        self.update_overlay(cx);
    }

    fn on_space_folder(
        &mut self,
        _: Entity<RemoteFolderPicker>,
        event: &PickerEvent,
        cx: &mut Context<'_, Self>,
    ) {
        self.remote_folders = None;
        self.update_overlay(cx);
        let PickerEvent::Chosen(path) = event else {
            return;
        };
        if self.first_run.is_some() {
            self.create_first_space(path, cx);
            return;
        }
        let path = path.clone();
        self.sidebar
            .update(cx, |sidebar, cx| sidebar.set_space_folder(path, cx));
    }

    fn show_remote_folders(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.remote_folders.is_some() {
            return;
        }
        let (signal, _) = crate::data_signal::callbacks(&self.requests, &self.data_queued);
        let opened = self
            .data
            .folder_browse(signal)
            .inspect_err(|error| tracing::warn!(%error, "the folder picker could not open"));
        let Ok(browse) = opened else {
            return;
        };
        let picker = cx.new(|_| RemoteFolderPicker::new(browse));
        self.subscriptions
            .push(cx.subscribe(&picker, Self::on_remote_folder));
        self.remote_folders = Some(picker);
        self.update_overlay(cx);
    }

    fn on_remote_folder(
        &mut self,
        _: Entity<RemoteFolderPicker>,
        event: &PickerEvent,
        cx: &mut Context<'_, Self>,
    ) {
        self.remote_folders = None;
        self.update_overlay(cx);
        if let PickerEvent::Chosen(path) = event {
            self.add_repo(path);
        }
    }
}

#[cfg(test)]
mod tests {
    use shell_data::FolderChoice;

    use super::*;

    #[test]
    fn only_a_space_on_a_host_opens_the_remote_picker() {
        assert!(!on_host(FolderChoice::Native));
        assert!(on_host(FolderChoice::OnHost));
    }
}
