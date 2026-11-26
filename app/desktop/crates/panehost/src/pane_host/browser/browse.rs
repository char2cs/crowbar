use layout::Layout;
use protocol::PaneId;
use webview::{Bounds, ParentWindow, Snapshot};

use crate::error::{Error, Result};
use crate::pane_host::PaneHost;
use crate::pane_host::browser::browse_action::BrowseAction;
use crate::pane_host::reconcile::{LayoutSnapshot, VisiblePane};
use crate::slot::Slot;
use crate::slot::is_browser;

const PARKED_WIDTH: f64 = 1280.0;
const PARKED_HEIGHT: f64 = 800.0;

fn parked(pane: &PaneId) -> VisiblePane {
    let bounds = Bounds {
        x: 0.0,
        y: 0.0,
        width: PARKED_WIDTH,
        height: PARKED_HEIGHT,
    };
    VisiblePane {
        pane: pane.clone(),
        bounds,
    }
}

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

    /// Gives a browser pane that has never been shown a hidden web view, so its page can load
    /// and be driven before anyone looks at it. A pane that already has a view is left alone;
    /// showing the pane later places the same view.
    ///
    /// # Errors
    /// [`Error::NotShown`] when `pane` is not a browser pane of `layout` and
    /// [`Error::WebView`] when the web view layer fails.
    pub fn wake_browser(
        &mut self,
        parent: ParentWindow,
        pane: &PaneId,
        layout: &Layout,
    ) -> Result<()> {
        self.track(
            &LayoutSnapshot::from_layout(layout, parent, Vec::new()),
            pane,
        );
        let runtime = self
            .panes
            .get(pane)
            .filter(|runtime| is_browser(&runtime.type_id))
            .ok_or_else(|| Error::NotShown(pane.clone()))?;
        if runtime.slot.is_some() {
            return Ok(());
        }
        let pane_type = self
            .registry
            .get(&runtime.type_id)
            .cloned()
            .ok_or_else(|| Error::NotShown(pane.clone()))?;
        self.assign_slot(parent, &pane_type, (&parked(pane), layout))
    }

    /// Whether a browser pane has a web view now, shown or hidden.
    #[must_use]
    pub fn has_view(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.browser_slot(pane).is_ok()
    }

    /// The pixels the page of a browser pane shows now; `None` when the backend cannot read
    /// them back.
    ///
    /// # Errors
    /// [`Error::NotShown`] when `pane` is not a browser pane with a web view and
    /// [`Error::WebView`] when the web view layer fails.
    pub fn capture_browser(
        &self,
        pane: &PaneId,
    ) -> Result<Option<Snapshot>> {
        Ok(self.browser_slot(pane)?.handle.snapshot()?)
    }
}

#[cfg(test)]
mod tests {
    use webview::{Call, Visibility};

    use browser::PageUrl;

    use super::*;
    use crate::catalog::PaneLifecycle;
    use crate::slot::{HostEvent, IpcMessage};
    use crate::test_support::{Fixture, browser_at, fixture, instance, pane_type};

    const WINDOW: ParentWindow = ParentWindow::X11(1);
    const PAGE: &str = "https://example.com/";

    fn browsing() -> Fixture {
        let mut f = fixture();
        let browser = pane_type("core.browser", PaneLifecycle::KeepAlive);
        f.host.registry_mut().register(browser).unwrap();
        f
    }

    fn id(pane: &str) -> PaneId {
        PaneId::new(pane).unwrap()
    }

    fn only_a_browser_pane_with_a_web_view_is_browsed_and_runs_scripts() {
        let mut f = browsing();
        let panes = [browser_at("b1", PAGE), instance("c1", "core.chat")];
        f.sync(&panes, &["b1", "c1"], Some("b1"));
        let go = BrowseAction::Go(PageUrl::parse(PAGE).unwrap());
        let script = BrowseAction::Script("return 1".into());
        for action in [&go, &BrowseAction::Back, &script] {
            assert!(f.host.browse(&id("b1"), action).is_ok(), "{action:?}");
        }
        for action in [&go, &BrowseAction::Reload] {
            let refused = f.host.browse(&id("c1"), action);
            assert!(matches!(refused, Err(Error::NotShown(_))), "{refused:?}");
        }
        let calls = f.mock.calls();
        let navigated = calls
            .iter()
            .filter(|call| matches!(call, Call::Navigate { url, .. } if url == PAGE));
        let ran = calls
            .iter()
            .any(|call| matches!(call, Call::Eval { script, .. } if script == "return 1"));
        assert_eq!(
            (navigated.count(), ran),
            (2, true),
            "placed, gone to: {calls:?}"
        );
    }

    fn a_browser_pane_gets_no_origin_so_the_relay_never_authorises_it() {
        let mut f = browsing();
        f.sync(&[browser_at("b1", PAGE)], &["b1"], Some("b1"));
        let calls = f.mock.calls();
        let Some(Call::Create { id, kind, .. }) = calls.first() else {
            panic!("no web view was created: {calls:?}");
        };
        assert_eq!(*kind, webview::Kind::Browser);
        assert!(f.host.origins.get(*id).is_none(), "{calls:?}");
    }

    fn a_press_on_a_browser_page_focuses_its_pane() {
        let mut f = browsing();
        f.sync(&[browser_at("b1", PAGE)], &["b1"], None);
        let text = r#"{"sdk":1,"kind":"shell.focus"}"#.into();
        let press = IpcMessage {
            pane: id("b1"),
            text,
        };
        assert_eq!(f.ipc(press), [HostEvent::PageFocused { pane: id("b1") }]);
    }

    fn a_browser_pane_nobody_has_shown_gets_a_hidden_view_and_keeps_it_when_shown() {
        let mut f = browsing();
        f.sync(&[instance("b1", "core.browser")], &[], None);
        assert!(!f.host.has_view(&id("b1")), "no view before the wake");
        for _ in 0..2 {
            f.host.wake_browser(WINDOW, &id("b1"), &f.layout).unwrap();
        }
        assert_eq!(f.host.web_view_count(), 1, "woken once");
        let created = f.mock.calls().into_iter().find_map(|call| match call {
            Call::Create { visibility, .. } => Some(visibility),
            _ => None,
        });
        assert_eq!(created, Some(Visibility::Hidden));
        let pixels = f.host.capture_browser(&id("b1")).unwrap();
        assert_eq!(pixels, None, "the mock shows nothing");
        f.sync(&[instance("b1", "core.browser")], &["b1"], Some("b1"));
        assert_eq!(
            f.host.web_view_count(),
            1,
            "showing the pane reuses the view"
        );
    }

    fn a_shown_browser_pane_has_no_view_until_it_has_an_address() {
        let mut f = browsing();
        f.sync(&[instance("b1", "core.browser")], &["b1"], Some("b1"));
        assert_eq!(f.host.web_view_count(), 0, "the pane fill shows, no page");
        f.sync(&[browser_at("b1", PAGE)], &["b1"], Some("b1"));
        let navigated = f.mock.calls().into_iter().any(|call| {
            matches!(call, Call::Navigate { url, .. } | Call::Create { url, .. } if url == PAGE)
        });
        assert!(
            f.host.has_view(&id("b1")) && navigated,
            "{:?}",
            f.mock.calls()
        );
    }

    fn only_a_browser_pane_of_the_layout_can_be_woken_and_captured() {
        let mut f = browsing();
        f.sync(&[instance("p1", "core.chat")], &[], None);
        for pane in [id("p1"), id("b1")] {
            let woken = f.host.wake_browser(WINDOW, &pane, &f.layout);
            assert!(matches!(woken, Err(Error::NotShown(_))), "{pane}");
        }
        let captured = f.host.capture_browser(&id("b1"));
        assert!(matches!(captured, Err(Error::NotShown(_))), "{captured:?}");
    }

    const CASES: &[fn()] = &[
        only_a_browser_pane_with_a_web_view_is_browsed_and_runs_scripts,
        a_browser_pane_gets_no_origin_so_the_relay_never_authorises_it,
        a_press_on_a_browser_page_focuses_its_pane,
        a_browser_pane_nobody_has_shown_gets_a_hidden_view_and_keeps_it_when_shown,
        a_shown_browser_pane_has_no_view_until_it_has_an_address,
        only_a_browser_pane_of_the_layout_can_be_woken_and_captured,
    ];

    #[test]
    fn only_browser_panes_are_browsed_woken_and_captured() {
        for case in CASES {
            case();
        }
    }
}
