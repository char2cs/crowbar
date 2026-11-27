use protocol::{Breakpoint, ShellFrame};

use crate::error::Result;
use crate::pane_host::PaneHost;

impl PaneHost {
    pub fn set_breakpoints(
        &mut self,
        cleared: Vec<Breakpoint>,
    ) -> Result<()> {
        if self.breakpoints.as_ref() == Some(&cleared) {
            return Ok(());
        }
        let frame = ShellFrame::Breakpoints {
            cleared: cleared.clone(),
        };
        self.breakpoints = Some(cleared);
        self.panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame))
    }

    pub(crate) fn breakpoints_context(&self) -> Option<ShellFrame> {
        let cleared = self.breakpoints.clone()?;
        Some(ShellFrame::Breakpoints { cleared })
    }
}

#[cfg(test)]
mod tests {
    use protocol::Breakpoint;

    use crate::test_support::*;

    fn sent(f: &Fixture) -> Vec<Vec<Breakpoint>> {
        frames_sent(&f.mock)
            .into_iter()
            .filter_map(|frame| match frame {
                protocol::ShellFrame::Breakpoints { cleared } => Some(cleared),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn a_pane_gets_the_breakpoints_on_mount_and_once_per_new_set() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        let (sm, lg) = (vec![Breakpoint::Sm], vec![Breakpoint::Sm, Breakpoint::Lg]);
        f.host.set_breakpoints(sm.clone()).unwrap();
        assert!(sent(&f).is_empty(), "not mounted yet");
        register(&mut f, "p1", "core.chat");
        f.host.set_breakpoints(sm.clone()).unwrap();
        f.host.set_breakpoints(lg.clone()).unwrap();
        assert_eq!(
            sent(&f),
            vec![sm, lg],
            "on mount, then only across a threshold"
        );
    }
}
