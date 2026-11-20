use crate::pane_chrome::PaneChrome;
use crate::pane_content::PaneContent;
use crate::pane_detach::PaneDetach;
use ideshell::TabLooks;

/// What the layout view asks of the shell about a pane; the shell implements it, the view never
/// looks a pane up itself.
pub trait PaneSource: PaneContent + PaneChrome + TabLooks + PaneDetach {}

impl<T: PaneContent + PaneChrome + TabLooks + PaneDetach> PaneSource for T {}

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
