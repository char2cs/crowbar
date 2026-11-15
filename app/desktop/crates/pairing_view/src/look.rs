//! Sizes and strings of the Add Host dialog.

use gpui::{Pixels, px};

pub(crate) const GAP: Pixels = px(8.0);
pub(crate) const TEXT: Pixels = px(13.0);
pub(crate) const LABEL_GAP: Pixels = px(4.0);
pub(crate) const FINGERPRINT_PAD: Pixels = px(10.0);
pub(crate) const FINGERPRINT_TINT: f32 = 0.08;

pub(crate) const TITLE: &str = "Add a host";
pub(crate) const NAME_PLACEHOLDER: &str = "Name, like Homelab";
pub(crate) const ADDRESS_PLACEHOLDER: &str = "homelab.example.com:3737";
pub(crate) const CODE_PLACEHOLDER: &str = "6-digit code";
