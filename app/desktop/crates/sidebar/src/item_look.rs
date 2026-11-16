//! How one context-menu row is dressed.

use ui::{IconName, Palette};

use crate::tone::Tone;

#[derive(Clone, Copy)]
pub(crate) struct ItemLook {
    pub(crate) icon: Option<IconName>,
    pub(crate) tone: Tone,
    pub(crate) active: bool,
    pub(crate) checked: Option<bool>,
    pub(crate) palette: &'static Palette,
}
