use gpui::Context;
use settings_ui::{Change, KEYBINDING_OVERRIDES, Key, SettingsBackend, Value};
use shell_keys::Keymap;

use super::ShellView;
use crate::shell_request::ShellRequest;

pub(super) fn keymap_for(stored: Option<&Value>) -> Keymap {
    match stored {
        Some(Value::Map(overrides)) => Keymap::with_overrides(overrides.iter()),
        _ => Keymap::defaults(),
    }
}

fn notify_when_overrides_move(
    requests: &async_channel::Sender<ShellRequest>,
    change: &Change,
) {
    let moved = matches!(change, Change::Setting(key, _) if key.as_str() == KEYBINDING_OVERRIDES);
    if moved && requests.try_send(ShellRequest::KeymapChanged).is_err() {
        tracing::debug!("the shell is closing; a keybinding change goes unseen");
    }
}

impl ShellView {
    pub(super) fn watch_keymap(&mut self) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        let requests = self.requests.clone();
        self.keymap_watch = Some(backend.subscribe(std::rc::Rc::new(move |change: &Change| {
            notify_when_overrides_move(&requests, change);
        })));
        self.reload_keymap();
    }

    pub(super) fn reload_keymap(&mut self) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        self.keymap = keymap_for(backend.get(&Key::new(KEYBINDING_OVERRIDES)).as_ref());
        self.state.set_keybindings(self.keymap.forwarded());
    }

    pub(super) fn on_keymap_changed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.reload_keymap();
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;

    use shell_keys::{Keystroke, ShellCommand};

    use super::*;

    fn press(chord_key: &str) -> Keystroke {
        Keystroke {
            primary: true,
            shift: false,
            alt: false,
            key: chord_key.to_owned(),
        }
    }

    #[test]
    fn stored_overrides_rebind_commands() {
        let stored = Value::Map(BTreeMap::from([(
            "tabs.new".to_owned(),
            "mod+k".to_owned(),
        )]));
        let keymap = keymap_for(Some(&stored));
        assert_eq!(keymap.resolve(&press("k")), Some(ShellCommand::NewTab));
        assert_eq!(keymap.resolve(&press("t")), None);
    }

    #[test]
    fn nothing_stored_or_a_wrong_shape_means_the_defaults() {
        for stored in [None, Some(Value::Text("x".to_owned()))] {
            assert_eq!(keymap_for(stored.as_ref()), Keymap::defaults());
        }
    }
}
