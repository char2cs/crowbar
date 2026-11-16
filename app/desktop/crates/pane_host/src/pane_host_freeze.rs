use protocol::PaneId;

use crate::error::Result;
use crate::frozen_pane::FrozenPane;
use crate::overlay::Overlay;
use crate::pane_host::PaneHost;
use crate::pane_runtime::PaneRuntime;

impl PaneHost {
    /// Whether an overlay currently hides the web views.
    #[must_use]
    pub fn is_overlaid(&self) -> bool {
        self.overlay == Overlay::Active
    }

    /// Hides every web view for an overlay and returns what the visible ones showed, so the
    /// shell can paint those pixels where the views were. Does nothing while already overlaid.
    ///
    /// # Errors
    /// [`Error::WebView`](crate::Error::WebView) when a web view cannot be driven.
    pub fn freeze(&mut self) -> Result<Vec<FrozenPane>> {
        if self.is_overlaid() {
            return Ok(Vec::new());
        }
        let frozen: Vec<FrozenPane> = self
            .panes
            .iter()
            .filter(|(_, runtime)| runtime.is_visible())
            .filter_map(|(id, runtime)| self.frozen_of(id, runtime))
            .collect();
        tracing::debug!(captured = frozen.len(), "web panes captured for an overlay");
        self.set_overlay(Overlay::Active)?;
        Ok(frozen)
    }

    fn frozen_of(
        &self,
        pane: &PaneId,
        runtime: &PaneRuntime,
    ) -> Option<FrozenPane> {
        let slot = runtime.slot.and_then(|id| self.pool.get(id))?;
        match slot.handle.snapshot() {
            Ok(snapshot) => snapshot.map(|snapshot| FrozenPane {
                pane: pane.clone(),
                snapshot,
            }),
            Err(error) => {
                tracing::warn!(%error, %pane, "a pane could not be captured for an overlay");
                None
            },
        }
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::*;

    #[test]
    fn freezing_hides_the_views_once_and_a_backend_without_pixels_returns_none() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        assert!(!f.host.is_overlaid(), "starts uncovered");
        assert!(
            f.host.freeze().unwrap().is_empty(),
            "the mock has no pixels"
        );
        assert!(f.host.is_overlaid(), "covered");
        assert!(
            f.host.freeze().unwrap().is_empty(),
            "second freeze is a no-op"
        );
    }
}
