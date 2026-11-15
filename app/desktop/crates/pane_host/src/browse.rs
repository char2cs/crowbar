use protocol::PaneId;

use crate::browse_action::BrowseAction;
use crate::browser_slot::is_browser;
use crate::error::{Error, Result};
use crate::pane_host::PaneHost;

impl PaneHost {
    /// Drives the web view of a browser pane. Only browser panes take part: anything else is
    /// refused so a pane type can never be navigated away from its own bundle.
    ///
    /// # Errors
    /// [`Error::NotShown`] when `pane` is not a browser pane with a web view and
    /// [`Error::WebView`] when the web view layer fails.
    pub fn browse(
        &self,
        pane: &PaneId,
        action: &BrowseAction,
    ) -> Result<()> {
        let runtime = self
            .panes
            .get(pane)
            .filter(|runtime| is_browser(&runtime.type_id));
        let slot = runtime
            .and_then(|runtime| runtime.slot)
            .and_then(|id| self.pool.get(id))
            .ok_or_else(|| Error::NotShown(pane.clone()))?;
        match action {
            BrowseAction::Go(url) => slot.handle.navigate(url.as_str())?,
            BrowseAction::Back => slot.handle.eval("history.back()")?,
            BrowseAction::Forward => slot.handle.eval("history.forward()")?,
            BrowseAction::Reload => slot.handle.eval("location.reload()")?,
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{fixture, instance};

    #[test]
    fn a_pane_without_a_browser_view_cannot_be_browsed() {
        let mut f = fixture();
        f.sync(&[instance("p1", "core.chat")], &["p1"], Some("p1"));
        let result = f
            .host
            .browse(&PaneId::new("p1").unwrap(), &BrowseAction::Reload);
        assert!(matches!(result, Err(Error::NotShown(_))), "{result:?}");
    }
}
