use crate::store::branch_import::entry::Entry;
use crate::store::branch_import::pr_link::PrLink;
use crate::store::settings_core::Loadable;

#[derive(Debug)]
pub(crate) struct Held {
    pub(crate) branches: Loadable<Vec<Entry>>,
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
