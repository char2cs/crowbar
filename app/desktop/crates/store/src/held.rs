//! What a branch import has heard from the daemon so far.

use crate::branch_entry::BranchEntry;
use crate::loadable::Loadable;
use crate::pr_link::PrLink;

#[derive(Debug)]
pub(crate) struct Held {
    pub(crate) branches: Loadable<Vec<BranchEntry>>,
    pub(crate) links: Vec<PrLink>,
}

impl Held {
    pub(crate) fn loading() -> Self {
        Self {
            branches: Loadable::Loading,
            links: Vec::new(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_new_hold_is_loading_with_no_links() {
        let held = Held::loading();
        assert!(matches!(held.branches, Loadable::Loading));
        assert_eq!(held.links.len(), 0);
    }
}
