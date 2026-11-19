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
            Err(error) => {
                return tracing::warn!(%error, "the theme could not be described to panes");
            },
        };
        if let Err(error) = self.host.set_theme(tokens) {
            tracing::warn!(%error, "sending the theme to panes failed");
        }
        let surface = crate::pane_theme::surface_color(theme);
        if let Err(error) = self.host.set_background(surface) {
            tracing::warn!(%error, "colouring the pane web views failed");
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

#[cfg(test)]
mod tests {
    use crate::test_support::{shell_rig, synced_chat};

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
