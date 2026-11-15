use std::path::PathBuf;

use model::Layout;

#[derive(Debug, PartialEq)]
pub(crate) struct LoadedLayout {
    pub(crate) layout: Layout,
    pub(crate) quarantined: Option<PathBuf>,
}
