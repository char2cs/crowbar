use protocol::{Breakpoint, ShellFrame};

use crate::error::Result;
use crate::pane_host::PaneHost;

impl PaneHost {
    /// Replaces the breakpoints the window clears and pushes them to mounted panes; a pane
    /// mounted later gets them on mount. An unchanged set sends nothing, so a resize between
    /// thresholds stays quiet.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
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
        f.host
            .set_breakpoints(Breakpoint::cleared_by(700.0))
            .unwrap();
        assert!(sent(&f).is_empty(), "not mounted yet");
        register(&mut f, "p1", "core.chat");
        f.host
            .set_breakpoints(Breakpoint::cleared_by(720.0))
            .unwrap();
        f.host
            .set_breakpoints(Breakpoint::cleared_by(1100.0))
            .unwrap();
        let (sm, lg) = (
            Breakpoint::cleared_by(700.0),
            Breakpoint::cleared_by(1100.0),
        );
        assert_eq!(
            sent(&f),
            vec![sm, lg],
            "on mount, then only across a threshold"
        );
    }
}
