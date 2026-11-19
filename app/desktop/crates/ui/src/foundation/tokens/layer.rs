//! Paint priority of overlays; larger values paint on top (`z-index` of the spec).

/// Popovers, selects, toasts.
pub const OVERLAY: usize = 50;
/// Tree drop indicator.
pub const DROP_LINE: usize = 60;
/// Window-wide modal layers (settings, onboarding, prompts), above the sidebar's own dialogs.
pub const MODAL: usize = 70;
/// Dropdown and context menu popups; above dialogs so menus opened inside them float.
pub const MENU: usize = 10_040;
/// Tooltips.
pub const TOOLTIP: usize = 99_999;
