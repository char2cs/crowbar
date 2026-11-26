use settings::{Change, Follow as _, Read as _};

use super::change_unseen_event::ChangeUnseenEvent;
use crate::shell::inbox::logged::emit;
use crate::shell::inbox::shell_request::ShellRequest;
use crate::shell::shell_view::ShellView;
use crate::shell::shell_view::settings::pane_settings::{follows, value_of, values_of};

mod pane_setting_event;
use pane_setting_event::PaneSettingEvent;

const MAX_OPEN_TABS: &str = "maxOpenTabs";
const AUTO_SAVE: &str = "autoSave";

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
        emit(&ChangeUnseenEvent::PaneSetting);
    }
}

fn store_one(
    backend: &impl settings::Backend,
    key: &protocol::SettingKey,
    json: &serde_json::Value,
) {
    let Some(value) = value_of(json) else {
        return emit(&PaneSettingEvent::UnknownShape { key });
    };
    if let Err(error) = backend.set(&settings::Key::new(key.as_str()), value) {
        emit(&PaneSettingEvent::Unstored { key, error: &error });
    }
}

impl ShellView {
    pub(crate) fn watch_pane_settings(&mut self) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let requests = self.requests.clone();
        self.watches.pane_settings =
            Some(backend.subscribe(std::rc::Rc::new(move |change: &Change| {
                notify_when_a_followed_setting_moves(&requests, change);
            })));
        self.push_pane_settings();
    }

    pub(crate) fn store_pane_settings(
        &mut self,
        writes: Vec<(protocol::SettingKey, serde_json::Value)>,
    ) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        for (key, json) in writes {
            store_one(&backend, &key, &json);
        }
    }

    pub(crate) fn autosaves(&mut self) -> bool {
        self.settings_backend()
            .and_then(|backend| backend.get(&settings::Key::new(AUTO_SAVE)))
            .and_then(|value| value.as_bool())
            .unwrap_or(false)
    }

    pub(crate) fn push_pane_settings(&mut self) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let limit = backend
            .get(&settings::Key::new(MAX_OPEN_TABS))
            .and_then(|value| value.as_number());
        self.state.set_open_limit(limit);
        match values_of(|key| backend.get(key)) {
            Ok(values) => self.state.set_pane_settings(values),
            Err(error) => tracing::warn!(%error, "the settings could not be described to panes"),
        }
    }
}

#[cfg(test)]
mod tests {
    use settings::{Key, Value};

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
