use std::collections::{HashMap, HashSet};

pub(crate) struct ParentChains<'a> {
    pub(crate) base: HashMap<&'a str, &'a str>,
    pub(crate) stop: HashSet<&'a str>,
    pub(crate) selected: HashSet<&'a str>,
    pub(crate) default_branch: &'a str,
}
