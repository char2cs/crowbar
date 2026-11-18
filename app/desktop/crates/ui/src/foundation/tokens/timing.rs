//! Durations of the transitions and animations in the spec.

use std::time::Duration;

/// Tailwind default transition: colours, opacity, shadows.
pub const DEFAULT: Duration = Duration::from_millis(150);
/// Dialog backdrop and popup, switch track, tab indicator.
pub const SLOW: Duration = Duration::from_millis(200);
/// Dropdown menu popup.
pub const MENU: Duration = Duration::from_millis(100);
/// Tooltip hover delay before it opens.
pub const TOOLTIP_DELAY: Duration = Duration::from_millis(150);
/// Toast success scale keyframes.
pub const TOAST_SUCCESS: Duration = Duration::from_millis(320);
/// Toast error shake keyframes.
pub const TOAST_ERROR: Duration = Duration::from_millis(280);
/// Scrollbar fade-out delay after the pointer leaves.
pub const SCROLLBAR_HIDE_DELAY: Duration = Duration::from_millis(300);
/// How often a text area scrolls one row while a selection is dragged past its edge.
pub const AUTOSCROLL: Duration = Duration::from_millis(50);
/// How long an automatic toast stays.
pub const TOAST_TIMEOUT: Duration = Duration::from_secs(5);
/// Skeleton shimmer cycle.
pub const SKELETON: Duration = Duration::from_secs(2);
/// Caret blink cycle.
pub const CARET: Duration = Duration::from_secs(1);
/// Spinner rotation cycle.
pub const SPIN: Duration = Duration::from_secs(1);
