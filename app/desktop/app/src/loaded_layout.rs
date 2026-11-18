use std::path::PathBuf;

use layout::Layout;

#[derive(Debug, PartialEq)]
pub(crate) struct LoadedLayout {
    pub(crate) layout: Layout,
    pub(crate) quarantined: Option<PathBuf>,
}
