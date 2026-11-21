use protocol::ShellFrame;

use crate::error::Result;
use crate::pane_host::PaneHost;

const UNZOOMED: u16 = 100;

impl PaneHost {
    /// Replaces the window's chat zoom, in percent, and pushes it to mounted panes; a pane
    /// mounted later gets it on mount. An unchanged zoom sends nothing.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a pane cannot be reached.
    pub fn set_chat_zoom(
        &mut self,
        percent: u16,
    ) -> Result<()> {
        if self.chat_zoom == Some(percent) {
            return Ok(());
        }
        self.chat_zoom = Some(percent);
        let frame = ShellFrame::ChatZoom { percent };
        self.panes
            .iter()
            .filter(|(_, runtime)| runtime.is_mounted())
            .try_for_each(|(id, _)| self.send(id, &frame))
    }

    /// The window's chat zoom in percent: 100 until the first change.
    #[must_use]
    pub fn chat_zoom(&self) -> u16 {
        self.chat_zoom.unwrap_or(UNZOOMED)
    }

    pub(crate) fn chat_zoom_context(&self) -> Option<ShellFrame> {
        let percent = self.chat_zoom?;
        Some(ShellFrame::ChatZoom { percent })
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::*;

    fn sent(f: &Fixture) -> Vec<u16> {
        frames_sent(&f.mock)
            .into_iter()
            .filter_map(|frame| match frame {
                protocol::ShellFrame::ChatZoom { percent } => Some(percent),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn a_pane_gets_the_chat_zoom_on_mount_and_once_per_change() {
        let mut f = fixture();
        assert_eq!(f.host.chat_zoom(), 100);
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        f.host.set_chat_zoom(110).unwrap();
        assert!(sent(&f).is_empty(), "not mounted yet");
        register(&mut f, "p1", "core.chat");
        f.host.set_chat_zoom(110).unwrap();
        f.host.set_chat_zoom(120).unwrap();
        assert_eq!(sent(&f), vec![110, 120], "on mount, then once per change");
        assert_eq!(f.host.chat_zoom(), 120);
    }
}
