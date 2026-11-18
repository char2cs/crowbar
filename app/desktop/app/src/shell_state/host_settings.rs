use panehost::Overlay;

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

    pub(crate) fn set_overlay(
        &mut self,
        overlay: Overlay,
    ) {
        if overlay == Overlay::Active {
            return self.freeze_panes();
        }
        if let Err(error) = self.host.set_overlay(overlay) {
            tracing::warn!(%error, "showing web views after an overlay failed");
        }
        self.frozen.replace(Vec::new());
    }

    fn freeze_panes(&mut self) {
        if self.host.is_overlaid() {
            return;
        }
        match self.host.freeze() {
            Ok(frozen) => self.frozen.replace(frozen),
            Err(error) => tracing::warn!(%error, "hiding web views for an overlay failed"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{shell_rig, synced_chat};

    #[test]
    fn an_overlay_hides_the_web_views_until_it_closes() {
        let mut rig = shell_rig();
        synced_chat(&mut rig);
        rig.shell.set_overlay(Overlay::Active);
        assert!(
            rig.shell.host().is_overlaid(),
            "hidden while the overlay is up"
        );
        rig.shell.set_overlay(Overlay::Inactive);
        assert!(!rig.shell.host().is_overlaid(), "shown again");
    }
}
