use gpui::{AnyElement, AnyView, App, SharedString, Window};
use model::TabId;
use protocol::PaneId;

/// What the layout view asks of the shell about a pane; the shell implements it, the view never
/// looks a pane up itself.
pub trait PaneSource {
    /// What fills the pane's content hole. The real shell returns an empty element, because the
    /// pane host's web view covers the hole.
    fn content(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement;

    /// Whether the pane is drawn by a native view that fills its whole content rectangle, so no
    /// web view and no hole are reserved for it.
    fn is_native(
        &self,
        _pane: &PaneId,
    ) -> bool {
        false
    }

    /// Height in logical pixels of the strip reserved at the top of the pane's content for
    /// [`PaneSource::chrome`]; the pane's hole starts below it.
    fn chrome_height(
        &self,
        _pane: &PaneId,
    ) -> f32 {
        0.0
    }

    /// The view drawn in the chrome strip, such as a browser address bar. Called only for panes
    /// whose [`PaneSource::chrome_height`] is above zero.
    fn chrome(
        &self,
        _pane: &PaneId,
        _window: &mut Window,
        _cx: &mut App,
    ) -> Option<AnyView> {
        None
    }

    /// The chat title shown on the drag ghost of the pane.
    fn label(
        &self,
        pane: &PaneId,
    ) -> SharedString;

    /// A tab id nobody uses yet and the title for the tab a detached pane opens.
    fn fresh_tab(
        &self,
        pane: &PaneId,
    ) -> (TabId, String);
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use super::PaneSource;
    use crate::test_support::{FakeSource, pane_id};

    #[test]
    fn every_detached_pane_gets_a_tab_id_nobody_uses() {
        let source: Rc<dyn PaneSource> = Rc::new(FakeSource::starting_at_first_tab());
        let first = source.fresh_tab(&pane_id("a"));
        let second = source.fresh_tab(&pane_id("a"));
        assert_ne!(first.0, second.0, "two detaches never share a tab id");
    }

    #[test]
    fn the_source_names_the_pane_and_by_default_adds_no_chrome() {
        let source: Rc<dyn PaneSource> = Rc::new(FakeSource::starting_at_first_tab());
        assert_eq!(
            source.label(&pane_id("a")).as_ref(),
            "a",
            "label comes from the source"
        );
        assert_eq!(
            source.chrome_height(&pane_id("a")),
            0.0,
            "no strip by default"
        );
        assert!(
            !source.is_native(&pane_id("a")),
            "hosted by a web view by default"
        );
    }
}
