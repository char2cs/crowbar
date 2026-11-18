//! How one context-menu row is dressed.

use ui::{IconName, Palette};

use crate::overlays::popup::check::Check;
use crate::overlays::popup::tone::Tone;

#[derive(Clone, Copy)]
pub(crate) struct ItemLook {
    pub(crate) icon: Option<IconName>,
    pub(crate) tone: Tone,
    pub(crate) active: bool,
    pub(crate) checked: Option<Check>,
    pub(crate) palette: &'static Palette,
}
