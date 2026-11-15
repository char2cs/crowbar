//! Typography scale: the Tailwind sizes and the `ui-text-*` sizes that follow the UI font setting.

use gpui::{Pixels, px};

/// Tailwind `text-xs` (12/16).
pub const XS: Pixels = px(12.0);
/// Tailwind `text-sm` (14/20).
pub const SM: Pixels = px(14.0);
/// Tailwind `text-base` (16/24).
pub const BASE: Pixels = px(16.0);
/// Tailwind `text-lg` (18/28).
pub const LG: Pixels = px(18.0);
/// Tailwind `text-xl` (20/28).
pub const XL: Pixels = px(20.0);
/// Line height of controls (`leading-5`).
pub const LINE_HEIGHT: Pixels = px(20.0);
