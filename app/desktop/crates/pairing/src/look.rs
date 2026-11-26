//! Sizes and strings of the Add Host dialog.

use gpui::{Pixels, px};

pub(crate) const GAP: Pixels = px(8.0);
pub(crate) const TEXT: Pixels = px(13.0);
pub(crate) const LABEL_GAP: Pixels = px(4.0);
pub(crate) const FINGERPRINT_PAD: Pixels = px(10.0);
pub(crate) const FINGERPRINT_TINT: f32 = 0.08;

pub(crate) const HINT_TEXT: Pixels = px(12.0);

// SPEC: https://github.com/char2cs/crowbar/blob/develop/web/src/features/hosts/components/add-host-dialog.tsx
pub(crate) const TITLE: &str = "Add remote host";
pub(crate) const NAME_PLACEHOLDER: &str = "homelab";
pub(crate) const ADDRESS_PLACEHOLDER: &str = "homelab.example.com or 1.2.3.4";
pub(crate) const ADDRESS_HINT: &str = "Uses port 3737 unless you add one, like :4000.";
pub(crate) const CODE_PLACEHOLDER: &str = "123456";
pub(crate) const CODE_HINT: &str = "The 6-digit code shown on the host.";
