//! What a branch import has heard from the daemon so far.

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
