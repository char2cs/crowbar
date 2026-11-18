use layout::Layout;
use protocol::{ExtensionStatus, KeyBinding, PaneId, ShellFrame, ThemeTokens};

use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::pane_host::overlay::Overlay;
use crate::pane_host::reconcile::Failures;

impl PaneHost {
    /// Hides or shows every pane for a native overlay. No pane is told.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::error::Error::WebView) when a web view cannot be driven.
    pub fn set_overlay(
        &mut self,
        overlay: Overlay,
    ) -> Result<()> {
        self.overlay = overlay;
        self.panes
            .values()
            .filter(|runtime| runtime.is_visible())
            .try_for_each(|runtime| self.set_native_visibility(runtime))
    }

    /// Replaces the active keybindings and pushes them to mounted panes.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::error::Error::WebView) when a pane cannot be reached.
    pub fn set_keybindings(
        &mut self,
        bindings: Vec<KeyBinding>,
    ) -> Result<()> {
        self.keybindings = bindings;
        let frame = ShellFrame::Keybindings {
            bindings: self.keybindings.clone(),
        };
        self.panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame))
    }

    /// Replaces the theme tokens and pushes them to mounted panes. Unchanged tokens send nothing.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::error::Error::WebView) when a pane cannot be reached.
    pub fn set_theme(
        &mut self,
        tokens: ThemeTokens,
    ) -> Result<()> {
        if self.theme.as_ref() == Some(&tokens) {
            return Ok(());
        }
        let frame = ShellFrame::Theme {
            tokens: tokens.clone(),
        };
        self.theme = Some(tokens);
        self.panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame))
    }

    /// Applies an extension's new status to its panes: errors in their frames while it is
    /// unavailable or disabled, a fresh `mount` with the saved state when it is back.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::error::Error::WebView) when a pane cannot be reached.
    pub fn extension_status(
        &mut self,
        status: &ExtensionStatus,
        layout: &Layout,
    ) -> Result<()> {
        let owned: Vec<PaneId> = self
            .panes
            .iter()
            .filter(|(_, runtime)| {
                runtime.type_id.extension().as_ref() == Some(status.extension_id())
            })
            .map(|(id, _)| id.clone())
            .collect();
        let mut failures = Failures::default();
        for pane in owned {
            failures.note(self.apply_status(&pane, status, layout));
        }
        failures.into_first().map_or(Ok(()), Err)
    }
}

#[cfg(test)]
mod tests {
    use protocol::{CommandId, KeyChord};
    use webview::{Call, Visibility};

    use super::*;
    use crate::test_support::*;

    fn last_visibility_is(
        f: &Fixture,
        expected: Visibility,
    ) -> bool {
        matches!(
            f.mock.calls().last(),
            Some(Call::SetVisibility { visibility, .. }) if *visibility == expected
        )
    }

    #[test]
    fn an_overlay_hides_web_views_without_telling_any_pane() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat")];
        f.sync(&panes, &["p1"], Some("p1"));
        register(&mut f, "p1", "core.chat");
        let before = frames_sent(&f.mock).len();
        f.host.set_overlay(Overlay::Active).unwrap();
        assert!(last_visibility_is(&f, Visibility::Hidden), "hidden");
        f.sync(&panes, &["p1"], Some("p1"));
        assert!(
            last_visibility_is(&f, Visibility::Hidden),
            "stays hidden while the overlay lasts"
        );
        f.host.set_overlay(Overlay::Inactive).unwrap();
        assert!(last_visibility_is(&f, Visibility::Shown), "shown again");
        assert_eq!(frames_sent(&f.mock).len(), before, "no frame was sent");
    }

    #[test]
    fn keybindings_reach_mounted_panes_now_and_later() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat")];
        f.sync(&panes, &["p1"], Some("p1"));
        let binding = KeyBinding::new(
            KeyChord::new("mod+k").unwrap(),
            CommandId::new("palette.open").unwrap(),
        );
        f.host.set_keybindings(vec![binding.clone()]).unwrap();
        assert!(
            frames_sent(&f.mock).is_empty(),
            "not mounted yet, nothing sent"
        );
        register(&mut f, "p1", "core.chat");
        assert!(
            frames_sent(&f.mock).contains(&ShellFrame::Keybindings {
                bindings: vec![binding]
            }),
            "sent on mount"
        );
    }
}
