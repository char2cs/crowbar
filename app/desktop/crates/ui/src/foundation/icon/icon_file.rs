//! One embedded Phosphor icon.

use crate::foundation::icon::icon_name::IconName;
use crate::foundation::icon::icon_weight::IconWeight;

#[derive(Debug)]
pub(crate) struct IconFile {
    pub(crate) name: IconName,
    pub(crate) weight: IconWeight,
    pub(crate) path: &'static str,
    pub(crate) bytes: &'static [u8],
}
