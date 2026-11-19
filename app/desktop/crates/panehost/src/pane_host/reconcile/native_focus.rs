use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::pane_host::lifecycle::stage::Stage;
use crate::pane_host::overlay::Overlay;
use crate::pane_host::reconcile::Keyboard;
use crate::slot::Slot;

impl PaneHost {
    /// Gives the keyboard back to whoever holds it in the shell's model once the window is
    /// active again: the window manager hands the focus to the window itself, never to a page.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::error::Error::WebView) when the web view cannot take or give
    /// back the keyboard.
    pub fn window_activated(&self) -> Result<()> {
        match self.keyboard_page() {
            Some(slot) => Ok(slot.handle.focus()?),
            None => self.hand_focus_to_native(),
        }
    }

    fn keyboard_page(&self) -> Option<&Slot> {
        let page_has_keys = self.keyboard == Keyboard::Pane && self.overlay == Overlay::Inactive;
        if !page_has_keys {
            return None;
        }
        let runtime = self
            .focused
            .as_ref()
            .and_then(|pane| self.panes.get(pane))?;
        if runtime.stage != Stage::Active {
            return None;
        }
        runtime.slot.and_then(|id| self.pool.get(id))
    }

    pub(crate) fn hand_focus_to_native(&self) -> Result<()> {
        let Some(focused) = self.focused.as_ref().and_then(|pane| self.panes.get(pane)) else {
            return Ok(());
        };
        if focused.slot.is_some() && self.keyboard == Keyboard::Pane {
            return Ok(());
        }
        let holder = self
            .panes
            .values()
            .filter(|runtime| runtime.is_visible())
            .find_map(|runtime| runtime.slot.and_then(|id| self.pool.get(id)));
        Ok(holder.map_or(Ok(()), |slot| slot.handle.focus_parent())?)
    }
}

#[cfg(test)]
mod tests {
    use webview::Call;

    use super::*;
    use crate::test_support::{fixture, instance, register};

    #[test]
    fn window_activation_gives_the_keyboard_back_to_its_owner() {
        let cases = [
            (
                "the page holds the keyboard",
                Keyboard::Pane,
                Overlay::Inactive,
                Some("focus"),
            ),
            (
                "the shell holds the keyboard",
                Keyboard::Shell,
                Overlay::Inactive,
                Some("focus_parent"),
            ),
            (
                "an overlay covers the page",
                Keyboard::Pane,
                Overlay::Active,
                None,
            ),
        ];
        for (name, keyboard, overlay, wanted) in cases {
            let mut f = fixture();
            let panes = [instance("p1", "core.chat")];
            f.sync(&panes, &["p1"], Some("p1"));
            register(&mut f, "p1", "core.chat");
            f.sync_with(&panes, &["p1"], (Some("p1"), keyboard));
            f.host.set_overlay(overlay).unwrap();
            let before = f.mock.calls().len();
            f.host.window_activated().unwrap();
            let focus_calls: Vec<&str> = f.mock.calls()[before..]
                .iter()
                .filter_map(focus_call)
                .collect();
            assert_eq!(focus_calls, Vec::from_iter(wanted), "{name}");
        }
    }

    fn focus_call(call: &Call) -> Option<&'static str> {
        match call {
            Call::Focus { .. } => Some("focus"),
            Call::FocusParent { .. } => Some("focus_parent"),
            _ => None,
        }
    }
}
