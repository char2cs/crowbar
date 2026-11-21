//! Pane layout of the active tab: the frame of every pane, the sashes between them, the accent
//! ring, the pane handle and the drag-and-drop zones.
//!
//! The model owns the layout. [`LayoutView`] renders the [`layout::Tab`] it is given and answers
//! pointer input with [`LayoutEvent::Apply`] commands; the owner applies them and gives the view
//! the new tab. Pane content areas are transparent holes: [`pane_holes`] gives the rectangles
//! the pane host places its web views in.
//!
//! Drag and drop is pure first: [`DropTarget`] turns a pointer position and a
//! [`layout::DragSubject`] into a target and a [`layout::Command`], [`SashGrab`] turns pointer
//! moves into resize commands, and the GPUI plumbing only forwards events to them.

mod branch_review;
mod chrome_context;
mod clock;
mod corners;
mod drop_hit;
mod drop_hover;
mod drop_target;
mod edge_contact;
mod handle_button;
mod insets;
mod layout_drag;
mod layout_event;
mod layout_input;
mod layout_render;
mod layout_view;
mod layoutview;
mod leaf_cell;
mod pane_actions;
mod pane_cell;
mod pane_chrome;
mod pane_content;
mod pane_detach;
mod pane_fades;
mod pane_frame;
mod pane_handle;
mod pane_hole;
mod pane_source;
mod pane_zone;
mod rect_tween;
mod ring_paint;
mod sash_grab;
mod sidebar_edge;
mod strip_actions;
mod strip_drop;
mod strip_head;
mod strip_index;
mod strip_input;
mod strip_key;
mod strip_keyboard;
mod strip_menu;
mod strip_review;
mod tab_seat;
mod test_support;

pub use branch_review::BranchReview;
pub use chrome_context::ChromeContext;
pub use clock::Clock;
pub use corners::Corners;
pub use drop_target::DropTarget;
pub use insets::Insets;
pub use layout_event::LayoutEvent;
pub use layout_view::LayoutView;
pub use layoutview::{pane_holes, pane_holes_with};
pub use pane_chrome::PaneChrome;
pub use pane_content::PaneContent;
pub use pane_detach::PaneDetach;
pub use pane_frame::PaneFrame;
pub use pane_hole::PaneHole;
pub use pane_source::PaneSource;
pub use pane_zone::PaneZone;
pub use sash_grab::SashGrab;
pub use sidebar_edge::SidebarEdge;
