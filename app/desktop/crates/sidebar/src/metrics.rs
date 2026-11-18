//! Pixel and ratio constants of the sidebar spec (`docs/plans/shell-spec/sidebar.md`).

pub(crate) const DEFAULT_WIDTH: f32 = 294.0;
pub(crate) const MIN_WIDTH: f32 = 250.0;
pub(crate) const MAX_WIDTH: f32 = 640.0;
pub(crate) const COLLAPSE_BELOW: f32 = MIN_WIDTH / 2.0;
pub(crate) const WINDOW_SHARE: f32 = 0.8;
pub(crate) const NARROW_BELOW: f32 = 640.0;
pub(crate) const KEY_STEP: f32 = 16.0;
pub(crate) const KEY_STEP_LARGE: f32 = 64.0;
pub(crate) const HEADER_HEIGHT: f32 = 42.0;
// PLATFORM: macOS draws its traffic lights over the top-left of the header.
#[cfg(target_os = "macos")]
pub(crate) const TRAFFIC_LIGHT_RESERVE: f32 = 72.0;
#[cfg(not(target_os = "macos"))]
pub(crate) const TRAFFIC_LIGHT_RESERVE: f32 = 0.0;
pub(crate) const SWITCHER_HEIGHT: f32 = 44.0;
pub(crate) const FOOT_HEIGHT: f32 = 40.0;
pub(crate) const ROW_HEIGHT: f32 = 36.0;
pub(crate) const ROW_MARGIN_X: f32 = 6.0;
pub(crate) const ROW_MARGIN_Y: f32 = 2.0;
pub(crate) const INDENT_STEP: f32 = 14.0;
pub(crate) const HANDLE_HIT: f32 = 8.0;
pub(crate) const PEEK_TRIGGER: f32 = 6.0;
pub(crate) const PEEK_INSET: f32 = 8.0;
pub(crate) const REMOVAL_DRAIN_SECONDS: u64 = 8;
pub(crate) const DRAG_THRESHOLD: f32 = 8.0;
pub(crate) const EDGE_BAND: f32 = 36.0;
pub(crate) const EDGE_SPEED_MAX: f32 = 14.0;
