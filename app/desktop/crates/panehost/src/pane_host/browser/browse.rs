use protocol::PaneId;

use crate::error::{Error, Result};
use crate::pane_host::PaneHost;
use crate::pane_host::browser::browse_action::BrowseAction;
use crate::slot::Slot;
use crate::slot::is_browser;

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
        let slot = self.browser_slot(pane)?;
        match action {
            BrowseAction::Go(url) => slot.handle.navigate(url.as_str())?,
            BrowseAction::Back => slot.handle.eval("history.back()")?,
            BrowseAction::Forward => slot.handle.eval("history.forward()")?,
            BrowseAction::Reload => slot.handle.eval("location.reload()")?,
            BrowseAction::Pick => slot.handle.eval(&browser::pick_script())?,
            BrowseAction::CancelPick => slot.handle.eval(&browser::cancel_pick_script())?,
            BrowseAction::Script(script) => slot.handle.eval(script)?,
        }
        Ok(())
    }

    pub(crate) fn browser_slot(
        &self,
        pane: &PaneId,
    ) -> Result<&Slot> {
        let runtime = self
            .panes
            .get(pane)
            .filter(|runtime| is_browser(&runtime.type_id));
        runtime
            .and_then(|runtime| runtime.slot)
            .and_then(|id| self.pool.get(id))
            .ok_or_else(|| Error::NotShown(pane.clone()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use webview::Call;

    use crate::catalog::PaneLifecycle;
    use crate::test_support::{browser_at, fixture, instance, pane_type};

    #[test]
    fn a_script_runs_in_the_page_of_a_browser_pane_only() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::KeepAlive))
            .unwrap();
        let panes = [
            browser_at("b1", "https://example.com/"),
            instance("p1", "core.chat"),
        ];
        f.sync(&panes, &["b1", "p1"], Some("b1"));
        let script = BrowseAction::Script("return 1".into());
        assert!(f.host.browse(&PaneId::new("b1").unwrap(), &script).is_ok());
        assert!(f.host.browse(&PaneId::new("p1").unwrap(), &script).is_err());
        let ran = f
            .mock
            .calls()
            .into_iter()
            .any(|call| matches!(call, Call::Eval { script, .. } if script == "return 1"));
        assert!(ran, "the script reached the web view");
    }

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
