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

mod geometry;
mod host;
mod layout_event;
mod layout_view;
mod layoutview;
mod test_support;

pub use geometry::chrome_context::ChromeContext;
pub use geometry::corners::Corners;
pub use geometry::insets::Insets;
pub use geometry::pane_frame::PaneFrame;
pub use geometry::sidebar_edge::SidebarEdge;
pub use host::branch_review::BranchReview;
pub use host::pane_chrome::PaneChrome;
pub use host::pane_content::PaneContent;
pub use host::pane_detach::PaneDetach;
pub use host::pane_hole::PaneHole;
pub use host::pane_source::PaneSource;
pub use layout_event::LayoutEvent;
pub use layout_view::LayoutView;
pub use layout_view::clock::Clock;
pub use layout_view::drag::drop_target::DropTarget;
pub use layout_view::drag::pane_zone::PaneZone;
pub use layout_view::drag::sash_grab::SashGrab;
pub use layoutview::{pane_holes, pane_holes_with};
