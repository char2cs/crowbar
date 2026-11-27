use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::pane_host::reconcile::Keyboard;
use crate::slot::Slot;

impl PaneHost {
    pub fn apply_keyboard(&self) -> Result<()> {
        match self.keyboard_page() {
            Some(slot) => Ok(slot.handle.focus()?),
            None => self.hand_focus_to_native(),
        }
    }

    pub(crate) fn settle_keyboard(
        &self,
        held_by: Keyboard,
    ) -> Result<()> {
        if held_by == self.keyboard {
            return self.hand_focus_to_native();
        }
        self.apply_keyboard()
    }

    fn keyboard_page(&self) -> Option<&Slot> {
        if self.keyboard != Keyboard::Pane {
            return None;
        }
        let runtime = self
            .focused
            .as_ref()
            .and_then(|pane| self.panes.get(pane))?;
        if !runtime.is_visible() {
            return None;
        }
        runtime.slot.and_then(|id| self.pool.get(id))
    }

    fn hand_focus_to_native(&self) -> Result<()> {
        let Some(focused) = self.focused.as_ref().and_then(|pane| self.panes.get(pane)) else {
            return Ok(());
        };
        if focused.slot.is_some() && self.keyboard == Keyboard::Pane {
            return Ok(());
        }
        let holder = self.pool.views().next().map(|(_, handle)| handle);
        Ok(holder.map_or(Ok(()), webview::Handle::focus_parent)?)
    }
}

#[cfg(test)]
mod tests {
    use webview::Call;

    use super::*;
    use crate::catalog::{PaneKind, PaneLifecycle};
    use crate::test_support::{browser_at, fixture, instance, pane_type, register};

    #[test]
    fn the_keyboard_goes_back_to_its_owner() {
        let cases = [
            ("the page holds the keyboard", Keyboard::Pane, Some("focus")),
            (
                "the shell holds the keyboard",
                Keyboard::Shell,
                Some("focus_parent"),
            ),
        ];
        for (name, keyboard, wanted) in cases {
            let mut f = fixture();
            let panes = [instance("p1", "core.chat")];
            f.sync(&panes, &["p1"], Some("p1"));
            register(&mut f, "p1", "core.chat");
            f.sync_with(&panes, &["p1"], (Some("p1"), keyboard));
            let before = f.mock.calls().len();
            f.host.apply_keyboard().unwrap();
            let focus_calls: Vec<&str> = f.mock.calls()[before..]
                .iter()
                .filter_map(focus_call)
                .collect();
            assert_eq!(focus_calls, Vec::from_iter(wanted), "{name}");
        }
    }

    #[test]
    fn the_active_page_takes_the_keyboard_back_when_the_shell_hands_it_over() {
        let cases = [
            ("handed from the shell", Keyboard::Shell, vec!["focus"]),
            ("already the page's", Keyboard::Pane, vec![]),
        ];
        for (name, held_by, wanted) in cases {
            let mut f = fixture();
            let panes = [instance("p1", "core.chat")];
            f.sync(&panes, &["p1"], Some("p1"));
            register(&mut f, "p1", "core.chat");
            f.sync_with(&panes, &["p1"], (Some("p1"), held_by));
            let before = f.mock.calls().len();
            f.sync_with(&panes, &["p1"], (Some("p1"), Keyboard::Pane));
            let focus_calls: Vec<&str> = f.mock.calls()[before..]
                .iter()
                .filter_map(focus_call)
                .collect();
            assert_eq!(focus_calls, wanted, "{name}");
        }
    }

    #[test]
    fn a_browser_page_takes_the_keyboard_without_registering() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::Parkable))
            .unwrap();
        let panes = [browser_at("b1", "https://example.com/")];
        f.sync_with(&panes, &["b1"], (Some("b1"), Keyboard::Pane));
        let before = f.mock.calls().len();
        f.host.apply_keyboard().unwrap();
        let focus_calls: Vec<&str> = f.mock.calls()[before..]
            .iter()
            .filter_map(focus_call)
            .collect();
        assert_eq!(focus_calls, ["focus"]);
    }

    #[test]
    fn a_native_pane_that_hides_the_page_takes_the_keyboard_from_it() {
        let mut f = fixture();
        let mut native = pane_type("core.native", PaneLifecycle::KeepAlive);
        native.kind = PaneKind::Native;
        f.host.registry_mut().register(native).unwrap();
        let panes = [instance("c1", "core.chat"), instance("n1", "core.native")];
        f.sync(&panes, &["c1"], Some("c1"));
        register(&mut f, "c1", "core.chat");
        f.sync_with(&panes, &["c1"], (Some("c1"), Keyboard::Pane));
        let before = f.mock.calls().len();
        f.sync(&panes, &["n1"], Some("n1"));
        let focus_calls: Vec<&str> = f.mock.calls()[before..]
            .iter()
            .filter_map(focus_call)
            .collect();
        assert_eq!(
            focus_calls,
            ["focus_parent"],
            "the hidden chat kept the keys the terminal opened over it"
        );
    }

    fn focus_call(call: &Call) -> Option<&'static str> {
        match call {
            Call::Focus { .. } => Some("focus"),
            Call::FocusParent { .. } => Some("focus_parent"),
            _ => None,
        }
    }
}
