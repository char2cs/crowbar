//! Pixel and ratio constants of the sidebar spec (`docs/plans/shell-spec/sidebar.md`).

pub(crate) const DEFAULT_WIDTH: f32 = 294.0;
pub(crate) const MIN_WIDTH: f32 = 250.0;
pub(crate) const MAX_WIDTH: f32 = 640.0;
pub(crate) const COLLAPSE_BELOW: f32 = MIN_WIDTH / 2.0;
pub(crate) const WINDOW_SHARE: f32 = 0.8;
pub(crate) const NARROW_BELOW: f32 = 640.0;
pub(crate) const KEY_STEP: f32 = 16.0;
pub(crate) const KEY_STEP_LARGE: f32 = 64.0;
/// Height of the sidebar header in pixels; the peek zone starts below it.
pub const HEADER_HEIGHT: f32 = 42.0;
pub(crate) const PEEK_TRIGGER: f32 = 6.0;
/// Gap in pixels between the window edge and the floating peek card.
pub const PEEK_INSET: f32 = 8.0;
pub(crate) const REMOVAL_DRAIN_SECONDS: u64 = 8;
