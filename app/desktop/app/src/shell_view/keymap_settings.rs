use chord::Keystroke;
use gpui::{App, Context, Window};
use keymap::{ContextualCommand, Keymap};
use settings::{
    Change, Follow as _, KEYBINDING_OVERRIDES, KEYBINDING_PRESET, Key, Read as _, Value,
    preset_overrides,
};

use super::ShellView;
use crate::shell_request::ShellRequest;

pub(super) fn keymap_for(
    stored: Option<&Value>,
    preset: Option<&Value>,
) -> Keymap {
    let mut bound = preset_overrides(preset.and_then(Value::as_text).unwrap_or_default());
    if let Some(Value::Map(overrides)) = stored {
        bound.extend(overrides.clone());
    }
    Keymap::with_overrides(bound.iter())
}

fn notify_when_overrides_move(
    requests: &async_channel::Sender<ShellRequest>,
    change: &Change,
) {
    let moved = matches!(
        change,
        Change::Setting(key, _) if key.as_str() == KEYBINDING_OVERRIDES || key.as_str() == KEYBINDING_PRESET
    );
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
        self.watches.keymap = Some(backend.subscribe(std::rc::Rc::new(move |change: &Change| {
            notify_when_overrides_move(&requests, change);
        })));
        self.reload_keymap();
    }

    pub(super) fn reload_keymap(&mut self) {
        let Some(backend) = self.settings_backend() else {
            return;
        };
        self.keymap = keymap_for(
            backend.get(&Key::new(KEYBINDING_OVERRIDES)).as_ref(),
            backend.get(&Key::new(KEYBINDING_PRESET)).as_ref(),
        );
    }

    fn live_contextual(
        &self,
        window: &Window,
        cx: &App,
    ) -> Vec<ContextualCommand> {
        let sidebar = self.sidebar.read(cx);
        [
            (
                ContextualCommand::DismissDrawer,
                sidebar.is_drawer_open(window),
            ),
            (ContextualCommand::SearchFiles, sidebar.shows_files(window)),
        ]
        .into_iter()
        .filter_map(|(command, live)| live.then_some(command))
        .collect()
    }

    pub(super) fn forward_bindings(
        &mut self,
        window: &Window,
        cx: &App,
    ) {
        let live = self.live_contextual(window, cx);
        self.state.set_keybindings(self.keymap.forwarded(&live));
    }

    pub(super) fn run_contextual(
        &mut self,
        command: ContextualCommand,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.live_contextual(window, cx).contains(&command) {
            return;
        }
        match command {
            ContextualCommand::DismissDrawer => {
                self.sidebar
                    .update(cx, sidebarview::Sidebar::dismiss_drawer);
            },
            ContextualCommand::SearchFiles => {
                self.sidebar
                    .update(cx, |sidebar, cx| sidebar.search_files(window, cx));
                self.state.keep_keyboard_in_shell();
                self.sync(cx);
            },
        }
    }

    pub(super) fn on_unbound_key(
        &mut self,
        stroke: &Keystroke,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(command) = ContextualCommand::of_stroke(stroke) else {
            return self.switch_project_on(stroke, cx);
        };
        if self.live_contextual(window, cx).contains(&command) {
            cx.stop_propagation();
            self.run_contextual(command, window, cx);
        }
    }

    pub(super) fn on_keymap_changed(
        &mut self,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.reload_keymap();
        self.forward_bindings(window, cx);
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;

    use chord::{Keystroke, Modifier};
    use keymap::ShellCommand;

    use super::*;

    fn press(chord_key: &str) -> Keystroke {
        Keystroke {
            modifiers: [Modifier::Primary].into_iter().collect(),
            key: chord_key.to_owned(),
            physical: None,
        }
    }

    #[test]
    fn stored_overrides_rebind_commands() {
        let stored = Value::Map(BTreeMap::from([(
            "tabs.new".to_owned(),
            "mod+k".to_owned(),
        )]));
        let keymap = keymap_for(Some(&stored), None);
        assert_eq!(keymap.resolve(&press("k")), Some(ShellCommand::NewTab));
        assert_eq!(keymap.resolve(&press("t")), None);
    }

    #[test]
    fn the_compact_preset_moves_reopen_and_an_override_beats_it() {
        let compact = Value::Text("compact".to_owned());
        let own = Value::Map(BTreeMap::from([(
            "tabs.reopenClosed".to_owned(),
            "mod+shift+k".to_owned(),
        )]));
        let cases = [
            (None, "r", Some(ShellCommand::ReopenClosedTab)),
            (Some(&own), "k", Some(ShellCommand::ReopenClosedTab)),
            (Some(&own), "r", None),
        ];
        for (stored, key, expected) in cases {
            let mut stroke = press(key);
            stroke.modifiers = [Modifier::Primary, Modifier::Shift].into_iter().collect();
            assert_eq!(
                keymap_for(stored, Some(&compact)).resolve(&stroke),
                expected,
                "{key}"
            );
        }
    }

    #[test]
    fn nothing_stored_or_a_wrong_shape_means_the_defaults() {
        for stored in [None, Some(Value::Text("x".to_owned()))] {
            assert_eq!(keymap_for(stored.as_ref(), None), Keymap::defaults());
        }
    }
}
