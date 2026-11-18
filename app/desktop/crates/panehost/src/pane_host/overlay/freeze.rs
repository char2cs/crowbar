use protocol::PaneId;
use webview::Snapshot;

use crate::error::Result;
use crate::pane_host::PaneHost;
use crate::pane_host::overlay::Overlay;
use crate::pane_host::overlay::frozen_pane::FrozenPane;

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
        let frozen = self.capture_visible();
        tracing::debug!(captured = frozen.len(), "web panes captured for an overlay");
        self.set_overlay(Overlay::Active)?;
        Ok(frozen)
    }

    fn capture_visible(&self) -> Vec<FrozenPane> {
        let shown: Vec<(&PaneId, webview::Id)> = self
            .panes
            .iter()
            .filter(|(_, runtime)| runtime.is_visible())
            .filter_map(|(id, runtime)| Some((id, self.pool.get(runtime.slot?)?.handle.id())))
            .collect();
        let ids: Vec<webview::Id> = shown.iter().map(|(_, id)| *id).collect();
        shown
            .into_iter()
            .zip(self.factory.snapshot_all(&ids))
            .filter_map(|((pane, _), captured)| frozen_of(pane, captured))
            .collect()
    }
}

fn frozen_of(
    pane: &PaneId,
    captured: webview::Result<Option<Snapshot>>,
) -> Option<FrozenPane> {
    match captured {
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

#[cfg(test)]
mod tests {
    use webview::Call;

    use crate::test_support::*;

    #[test]
    fn every_visible_pane_is_captured_in_one_batch() {
        let mut f = fixture();
        let panes = [instance("p1", "core.chat"), instance("p2", "core.chat")];
        f.sync(&panes, &["p1", "p2"], Some("p1"));
        f.host.freeze().unwrap();
        let batches: Vec<usize> = f
            .mock
            .calls()
            .into_iter()
            .filter_map(|call| match call {
                Call::SnapshotAll { ids } => Some(ids.len()),
                _ => None,
            })
            .collect();
        assert_eq!(batches, [2], "one request for both views");
    }

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
