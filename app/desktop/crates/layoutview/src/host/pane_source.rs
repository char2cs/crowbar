use crate::host::branch_review::BranchReview;
use crate::host::pane_chrome::PaneChrome;
use crate::host::pane_content::PaneContent;
use crate::host::pane_detach::PaneDetach;
use ideshell::TabLooks;

/// What the layout view asks of the shell about a pane; the shell implements it, the view never
/// looks a pane up itself.
pub trait PaneSource: PaneContent + PaneChrome + TabLooks + PaneDetach + BranchReview {}

impl<T: PaneContent + PaneChrome + TabLooks + PaneDetach + BranchReview> PaneSource for T {}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use super::PaneSource;
    use crate::test_support::{FakeSource, pane_id};

    #[test]
    fn the_source_names_the_pane_and_by_default_adds_no_chrome() {
        let source: Rc<dyn PaneSource> = Rc::new(FakeSource::starting_at_first_tab());
        let pane = pane_id("a");
        let answers = (
            source.label(&pane).to_string(),
            source.chrome_height(&pane),
            source.is_native(&pane),
        );
        assert_eq!(
            answers,
            ("a".to_owned(), 0.0, false),
            "label, strip, web view"
        );
    }
}
