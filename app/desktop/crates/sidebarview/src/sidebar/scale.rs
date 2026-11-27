use gpui::{Pixels, px};

pub(crate) const SPACE_XS: Pixels = px(4.0);
pub(crate) const SPACE_SM: Pixels = px(6.0);
pub(crate) const SPACE_MD: Pixels = px(8.0);
pub(crate) const SPACE_LG: Pixels = px(12.0);
pub(crate) const SPACE_XL: Pixels = px(24.0);

pub(crate) const TEXT_MICRO: Pixels = px(10.0);
pub(crate) const TEXT_CAPTION: Pixels = px(11.0);
pub(crate) const TEXT_SMALL: Pixels = px(12.0);
pub(crate) const TEXT_BODY: Pixels = px(13.0);
pub(crate) const TEXT_LARGE: Pixels = px(14.0);

pub(crate) const ICON_TINY: Pixels = px(10.0);
pub(crate) const ICON_SMALL: Pixels = px(12.0);
pub(crate) const ICON_MEDIUM: Pixels = px(14.0);
pub(crate) const ICON_LARGE: Pixels = px(16.0);

// PLATFORM: macOS draws its traffic lights over the top-left of the header.
#[cfg(target_os = "macos")]
pub(crate) const TRAFFIC_LIGHT_RESERVE: f32 = 72.0;
#[cfg(not(target_os = "macos"))]
pub(crate) const TRAFFIC_LIGHT_RESERVE: f32 = 0.0;
pub(crate) const HEADER_HEIGHT: f32 = 42.0;
pub(crate) const PEEK_INSET: f32 = 8.0;
pub(crate) const SWITCHER_HEIGHT: f32 = 44.0;
pub(crate) const FOOT_HEIGHT: f32 = 40.0;
pub(crate) const ROW_HEIGHT: f32 = 36.0;
pub(crate) const ROW_MARGIN_X: f32 = 6.0;
pub(crate) const ROW_MARGIN_Y: f32 = 2.0;
pub(crate) const INDENT_STEP: f32 = 14.0;
pub(crate) const HANDLE_HIT: f32 = 8.0;
