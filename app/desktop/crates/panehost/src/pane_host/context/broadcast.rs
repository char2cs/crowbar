use layout::Layout;
use protocol::{ExtensionStatus, KeyBinding, PaneId, ShellFrame, ThemeTokens};

use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::pane_host::reconcile::Failures;

impl PaneHost {
    pub fn set_keybindings(
        &mut self,
        bindings: Vec<KeyBinding>,
    ) -> Result<()> {
        if self.keybindings == bindings {
            return Ok(());
        }
        self.keybindings = bindings;
        let frame = ShellFrame::Keybindings {
            bindings: self.keybindings.clone(),
        };
        self.panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame))
    }

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

    use super::*;
    use crate::test_support::*;

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
                bindings: vec![binding.clone()]
            }),
            "sent on mount"
        );
        let before = frames_sent(&f.mock).len();
        f.host.set_keybindings(vec![binding]).unwrap();
        assert_eq!(
            frames_sent(&f.mock).len(),
            before,
            "the same bindings again send nothing"
        );
    }
}
