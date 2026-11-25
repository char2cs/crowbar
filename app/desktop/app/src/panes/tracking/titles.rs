use std::cell::RefCell;
use std::collections::BTreeMap;
use std::rc::Rc;

use protocol::{PaneId, PaneTitle};

#[derive(Debug, Clone, Default)]
pub(crate) struct Titles {
    entries: Rc<RefCell<BTreeMap<PaneId, PaneTitle>>>,
}

impl Titles {
    pub(crate) fn set(
        &self,
        pane: PaneId,
        title: PaneTitle,
    ) {
        self.entries.borrow_mut().insert(pane, title);
    }

    pub(crate) fn get(
        &self,
        pane: &PaneId,
    ) -> Option<PaneTitle> {
        self.entries.borrow().get(pane).cloned()
    }

    pub(crate) fn retain(
        &self,
        keep: impl Fn(&PaneId) -> bool,
    ) {
        self.entries.borrow_mut().retain(|pane, _| keep(pane));
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn clones_share_titles_and_retain_prunes() {
        let titles = Titles::default();
        let copy = titles.clone();
        let (a, b) = (PaneId::new("a").unwrap(), PaneId::new("b").unwrap());
        titles.set(a.clone(), PaneTitle::new("A").unwrap());
        copy.set(b.clone(), PaneTitle::new("B").unwrap());
        assert_eq!(
            copy.get(&a).map(|t| t.to_string()),
            Some("A".into()),
            "shared"
        );
        titles.retain(|pane| *pane == b);
        assert!(copy.get(&a).is_none(), "pruned");
        assert!(titles.get(&b).is_some(), "kept");
    }
}
