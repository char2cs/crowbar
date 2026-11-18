use layout::Layout;
use protocol::PaneId;
use webview::{Bounds, ParentWindow, Snapshot};

use crate::browser_slot::is_browser;
use crate::error::{Error, Result};
use crate::pane_host::PaneHost;
use crate::visible_pane::VisiblePane;

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
        self.track(layout, pane);
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

    use super::*;
    use crate::pane_lifecycle::PaneLifecycle;
    use crate::test_support::{fixture, instance, pane_type};

    const WINDOW: ParentWindow = ParentWindow::X11(1);

    fn browser_id() -> PaneId {
        PaneId::new("b1").unwrap()
    }

    #[test]
    fn a_browser_pane_nobody_has_shown_gets_a_hidden_view_at_its_address_and_keeps_it_when_shown() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::KeepAlive))
            .unwrap();
        f.sync(&[instance("b1", "core.browser")], &[], None);
        assert_eq!(f.host.web_view_count(), 0, "no view before the wake");
        f.host
            .wake_browser(WINDOW, &browser_id(), &f.layout)
            .unwrap();
        f.host
            .wake_browser(WINDOW, &browser_id(), &f.layout)
            .unwrap();
        assert_eq!(f.host.web_view_count(), 1, "woken once");
        let created = f.mock.calls().into_iter().find_map(|call| match call {
            Call::Create { visibility, .. } => Some(visibility),
            _ => None,
        });
        assert_eq!(created, Some(Visibility::Hidden));
        f.sync(&[instance("b1", "core.browser")], &["b1"], Some("b1"));
        assert_eq!(
            f.host.web_view_count(),
            1,
            "showing the pane reuses the view"
        );
    }

    #[test]
    fn only_a_browser_pane_of_the_layout_can_be_woken_and_captured() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::KeepAlive))
            .unwrap();
        f.sync(&[instance("p1", "core.chat")], &[], None);
        let chat = PaneId::new("p1").unwrap();
        let cases = [chat, browser_id()];
        for pane in cases {
            let woken = f.host.wake_browser(WINDOW, &pane, &f.layout);
            assert!(matches!(woken, Err(Error::NotShown(_))), "{pane}");
        }
        assert!(matches!(
            f.host.capture_browser(&browser_id()),
            Err(Error::NotShown(_))
        ));
    }

    #[test]
    fn a_woken_view_is_asked_for_its_pixels_without_failing() {
        let mut f = fixture();
        f.host
            .registry_mut()
            .register(pane_type("core.browser", PaneLifecycle::KeepAlive))
            .unwrap();
        f.sync(&[instance("b1", "core.browser")], &[], None);
        f.host
            .wake_browser(WINDOW, &browser_id(), &f.layout)
            .unwrap();
        assert_eq!(
            f.host.capture_browser(&browser_id()).unwrap(),
            None,
            "the mock shows nothing"
        );
    }
}
