//! What the open-PR chains say about which parents an import has to create.

use std::collections::{HashMap, HashSet};

pub(crate) struct ParentChains<'a> {
    pub(crate) base: HashMap<&'a str, &'a str>,
    pub(crate) stop: HashSet<&'a str>,
    pub(crate) selected: HashSet<&'a str>,
    pub(crate) default_branch: &'a str,
}
