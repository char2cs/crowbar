//! The documents shown as their source rather than their rich view, by document URI: the view
//! belongs to the document, as develop's markdown view store keys it by buffer, so every pane on
//! a document shows the same view, and a document that closes opens rich again.

use std::cell::RefCell;
use std::collections::BTreeSet;

use super::pane_view::PaneView;

#[derive(Default)]
pub(super) struct SourceShown {
    uris: RefCell<BTreeSet<String>>,
}

impl SourceShown {
    pub(super) fn contains(
        &self,
        uri: &str,
    ) -> bool {
        self.uris.borrow().contains(uri)
    }

    pub(super) fn show(
        &self,
        uri: String,
        view: PaneView,
    ) {
        let mut uris = self.uris.borrow_mut();
        if view == PaneView::Source {
            uris.insert(uri);
            return;
        }
        uris.remove(&uri);
    }

    pub(super) fn forget(
        &self,
        uri: &str,
    ) {
        self.uris.borrow_mut().remove(uri);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const URI: &str = "crowbar-workspace://w1/README.md";

    #[test]
    fn a_document_shows_its_source_until_it_goes_back_to_rich_or_closes() {
        let shown = SourceShown::default();
        shown.show(URI.to_owned(), PaneView::Source);
        assert!(shown.contains(URI), "source");
        shown.show(URI.to_owned(), PaneView::Rich);
        assert!(!shown.contains(URI), "rich again");
        shown.show(URI.to_owned(), PaneView::Source);
        shown.forget(URI);
        assert!(!shown.contains(URI), "closed");
    }
}
