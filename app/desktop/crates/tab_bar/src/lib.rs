//! The view-tab strip of the Crowbar shell: one pill per model tab with a close button, drag to
//! reorder, drop a chat on a pill to merge it and a re-open toggle for a collapsed sidebar.
//!
//! The model owns the tabs. [`TabBar`] renders the [`TabView`]s it is given and answers input
//! with [`TabBarEvent::Apply`] commands. The drag logic is pure: [`StripDrag`] moves a tab along
//! the strip and [`TabDrop`] turns a chat dropped from elsewhere into commands; the GPUI plumbing
//! only forwards pointer events to them.

mod drag_session;
mod incoming_drop;
mod member_view;
mod new_tab;
mod press;
mod settle;
mod shell_look;
mod sidebar_glyph;
mod sidebar_state;
mod span;
mod strip_drag;
mod strip_metrics;
mod tab_bar;
mod tab_bar_event;
mod tab_bar_geometry;
mod tab_bar_incoming;
mod tab_bar_menu;
mod tab_bar_pointer;
mod tab_bar_render;
mod tab_drop;
mod tab_shell;
mod tab_view;
mod test_support;

pub use member_view::MemberView;
pub use new_tab::NewTab;
pub use shell_look::ShellLook;
pub use sidebar_state::SidebarState;
pub use span::Span;
pub use strip_drag::StripDrag;
pub use tab_bar::TabBar;
pub use tab_bar_event::TabBarEvent;
pub use tab_drop::TabDrop;
pub use tab_view::TabView;
