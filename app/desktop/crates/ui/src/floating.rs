//! Everything painted above the window: portals, popovers, dialogs, menus,
//! tooltips and drag ghosts.

mod dialog;
mod drag_ghost;
mod layers;
mod menu;
mod native_alert;
mod placement;
mod popover;
mod popup;
mod portal;
mod tab_preview;
mod tooltip;
mod tooltip_claim;
mod tooltip_side;

pub(crate) use dialog::bindings as dialog_bindings;
pub use dialog::{Dialog, DialogExtent, DialogSize, DialogStyle, HeaderFlow};
pub use drag_ghost::DragGhost;
pub use layers::{
    FloatHost, FloatRegion, Shaped, holds_keys, install_float_host, open_float_window,
};
pub(crate) use menu::bindings as menu_bindings;
pub use menu::{
    ContextMenu, DropdownMenu, Menu, MenuAction, MenuEdge, MenuItem, MenuPath, NativeEntry,
    NativeMenu, NativeRequest, PopupMenu, Select, on_context_click,
};
pub use native_alert::{AlertButton, AlertRequest, AlertRole, AskAlert, NativeAlert};
pub use placement::Placement;
pub use popover::Popover;
pub(crate) use popover::bindings as popover_bindings;
pub use popup::{click_away, popup_at};
pub use portal::{Float, Portal};
pub use tab_preview::TabPreview;
pub use tooltip::Tooltip;
pub use tooltip_side::TooltipSide;
