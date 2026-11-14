//! Paint priority of overlays; larger values paint on top (`z-index` of the spec).

/// Pane content box.
pub const CONTENT: usize = 1;
/// Sidebar split handle, pane sash, dragged tab.
pub const HANDLE: usize = 10;
/// Pane handle tab and narrow-width sidebar scrim.
pub const PANE_HANDLE: usize = 30;
/// Sidebar peek card, drop tint.
pub const PEEK: usize = 40;
/// Dialog backdrop and viewport, popovers, selects, toasts.
pub const OVERLAY: usize = 50;
/// Tree drop indicator.
pub const DROP_LINE: usize = 60;
/// Dropdown and context menu popups; above dialogs so menus opened inside them float.
pub const MENU: usize = 10_040;
/// Tooltips.
pub const TOOLTIP: usize = 99_999;
