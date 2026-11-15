use settings_ui::{Change, SettingsBackend};

use super::ShellView;
use crate::pane_settings::{follows, values_of};
use crate::shell_request::ShellRequest;

fn notify_when_a_followed_setting_moves(
    requests: &async_channel::Sender<ShellRequest>,
    change: &Change,
) {
    let moved = matches!(change, Change::Setting(key, _) if follows(key));
    if moved
        && requests
            .try_send(ShellRequest::PaneSettingsChanged)
            .is_err()
    {
        tracing::debug!("the shell is closing; a pane setting change goes unseen");
    }
}

impl ShellView {
    pub(super) fn watch_pane_settings(&mut self) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let requests = self.requests.clone();
        self.pane_settings_watch =
            Some(backend.subscribe(std::rc::Rc::new(move |change: &Change| {
                notify_when_a_followed_setting_moves(&requests, change);
            })));
        self.push_pane_settings();
    }

    pub(super) fn push_pane_settings(&mut self) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        match values_of(|key| backend.get(key)) {
            Ok(values) => self.state.set_pane_settings(values),
            Err(error) => tracing::warn!(%error, "the settings could not be described to panes"),
        }
    }
}

#[cfg(test)]
mod tests {
    use settings_ui::{Key, Value};

    use super::*;

    #[test]
    fn only_a_followed_setting_queues_a_push() {
        let (requests, queue) = async_channel::unbounded();
        for (name, queued) in [
            ("wordWrap", true),
            ("theme", false),
            ("sidebarPosition", false),
        ] {
            let change = Change::Setting(Key::new(name), Value::Bool(true));
            notify_when_a_followed_setting_moves(&requests, &change);
            assert_eq!(queue.try_recv().is_ok(), queued, "{name}");
        }
    }
}
