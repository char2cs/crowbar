//! Context menus, dropdowns and selects; all of them open the desktop's own menu.

mod menu_item;
mod native_menu;
mod trigger;

pub use menu_item::{MenuAction, MenuItem};
pub use native_menu::{
    MenuEdge, MenuPath, NativeEntry, NativeMenu, NativeRequest, PopupMenu, on_context_click,
};
pub use trigger::{ContextMenu, DropdownMenu, Select};
