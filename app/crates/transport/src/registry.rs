use std::collections::HashMap;

use crate::live::Live;

#[derive(Default)]
pub(crate) struct Registry {
    pub(crate) next: u64,
    pub(crate) live: HashMap<u64, Live>,
}
