use super::ShellState;

impl ShellState {
    pub(crate) fn set_keybindings(
        &mut self,
        bindings: Vec<protocol::KeyBinding>,
    ) {
        if let Err(error) = self.host.set_keybindings(bindings) {
            tracing::warn!(%error, "pushing the keybindings to the panes failed");
        }
    }

    pub(crate) fn set_theme(
        &mut self,
        theme: ui::Theme,
    ) {
        let tokens = match crate::pane_theme::tokens_of(theme) {
            Ok(tokens) => tokens,
            Err(error) => return warn_theme_undescribed(&error),
        };
        if let Err(error) = self.host.set_theme(tokens) {
            warn_theme_unsent(&error);
        }
        let surface = crate::pane_theme::surface_color(theme);
        if let Err(error) = self.host.set_background(surface) {
            warn_views_uncoloured(&error);
        }
    }

    pub(crate) fn set_window_width(
        &mut self,
        width: f32,
    ) {
        let cleared = ui::Breakpoint::cleared_by(width)
            .into_iter()
            .map(crate::pane_breakpoint::wire_breakpoint)
            .collect();
        if let Err(error) = self.host.set_breakpoints(cleared) {
            tracing::warn!(%error, "sending the window's breakpoints to panes failed");
        }
    }

    pub(crate) fn set_pane_settings(
        &mut self,
        values: protocol::SettingValues,
    ) {
        if let Err(error) = self.host.set_settings(values) {
            tracing::warn!(%error, "sending the settings to panes failed");
        }
    }
}

fn warn_theme_undescribed(error: &protocol::Error) {
    tracing::warn!(%error, "the theme could not be described to panes");
}

fn warn_theme_unsent(error: &panehost::Error) {
    tracing::warn!(%error, "sending the theme to panes failed");
}

fn warn_views_uncoloured(error: &panehost::Error) {
    tracing::warn!(%error, "colouring the pane web views failed");
}

#[cfg(test)]
mod tests {
    use crate::test_support::{shell_rig, synced_chat};

    #[test]
    fn a_mounted_pane_learns_the_breakpoints_the_window_width_clears() {
        let mut rig = shell_rig();
        let pane = synced_chat(&mut rig);
        let register = protocol::PaneFrame::Register {
            type_id: crate::core_panes::chat_type().unwrap(),
            sdk: protocol::SDK_VERSION,
        };
        let text = protocol::encode(&register).unwrap();
        rig.shell.handle_ipc(panehost::IpcMessage { pane, text });
        rig.shell.set_window_width(1100.0);
        let frame = r#"{\"sdk\":1,\"kind\":\"breakpoints\",\"cleared\":[\"sm\",\"md\",\"lg\"]}"#;
        let told = rig.mock.calls().into_iter().any(|call| match call {
            webview::Call::Eval { script, .. } => script.contains(frame),
            _ => false,
        });
        assert!(told, "{:?}", rig.mock.calls());
    }

    #[test]
    fn a_theme_change_recolours_the_live_pane_views() {
        let mut rig = shell_rig();
        synced_chat(&mut rig);
        let theme = ui::Theme::default();
        rig.shell.set_theme(theme);
        let painted: Vec<_> = rig
            .mock
            .calls()
            .into_iter()
            .filter_map(|call| match call {
                webview::Call::SetBackground { color, .. } => Some(color),
                _ => None,
            })
            .collect();
        assert_eq!(painted, vec![crate::pane_theme::surface_color(theme)]);
    }
}
