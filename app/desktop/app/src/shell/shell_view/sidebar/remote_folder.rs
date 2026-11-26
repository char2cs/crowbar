use folderpicker::PickerEvent;
use gpui::{AppContext as _, Context, Entity};

use crate::shell::inbox::data_signal;
use crate::shell::shell_view::ShellView;

impl ShellView {
    pub(crate) fn choose_space_folder(
        &mut self,
        host: &str,
        cx: &mut Context<'_, Self>,
    ) {
        if self.remote_folders.is_some() {
            return;
        }
        let (signal, _) = data_signal::callbacks(&self.requests, &self.data_queued);
        let opened = self
            .data
            .folder_browse_on(host, signal)
            .inspect_err(|error| tracing::warn!(%error, "the folder picker could not open"));
        let Ok(browse) = opened else {
            return;
        };
        let picker = cx.new(|_| folderpicker::View::new(browse));
        self.subscriptions
            .push(cx.subscribe(&picker, Self::on_space_folder));
        self.remote_folders = Some(picker);
        cx.notify();
    }

    fn on_space_folder(
        &mut self,
        _: Entity<folderpicker::View>,
        event: &PickerEvent,
        cx: &mut Context<'_, Self>,
    ) {
        self.remote_folders = None;
        cx.notify();
        let PickerEvent::Chosen(path) = event else {
            return;
        };
        let path = path.clone();
        self.sidebar
            .update(cx, |sidebar, cx| sidebar.set_space_folder(path, cx));
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::shell::shell_view::tests::open_shell;

    #[gpui::test]
    fn an_unknown_host_opens_no_folder_picker(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let shown = opened.shell.update(&mut opened.window, |shell, cx| {
            shell.choose_space_folder("ghost", cx);
            shell.remote_folders.is_some()
        });
        assert!(!shown, "no daemon answers for that host");
    }
}
