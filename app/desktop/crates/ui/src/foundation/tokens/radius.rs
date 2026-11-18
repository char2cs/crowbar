//! Corner radii (`--radius` 10px scale of the spec).

use gpui::{Pixels, px};

/// `rounded-sm`.
pub const SM: Pixels = px(6.0);
/// `rounded-md`.
pub const MD: Pixels = px(8.0);
/// `rounded-lg`, the control radius.
pub const LG: Pixels = px(10.0);
/// `rounded-xl`.
pub const XL: Pixels = px(14.0);
/// `rounded-2xl`, dialogs and cards.
pub const XXL: Pixels = px(18.0);
/// `rounded-full`.
pub const FULL: Pixels = px(9999.0);
