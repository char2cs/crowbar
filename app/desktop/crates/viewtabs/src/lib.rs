//! The view-tab strip of the Crowbar shell: one pill per model tab with a close button, drag to
//! reorder, drop a chat on a pill to merge it and a re-open toggle for a collapsed sidebar.
//!
//! The model owns the tabs. [`TabBar`] renders the [`TabView`]s it is given and answers input
//! with [`Event::Apply`] commands. The drag logic is pure: [`StripDrag`] moves a tab along
//! the strip and [`TabDrop`] turns a chat dropped from elsewhere into commands; the GPUI plumbing
//! only forwards pointer events to them.

mod event;
mod gesture;
mod pill;
mod span;
mod strip_metrics;
mod tab_bar;
mod test_support;

pub use event::Event;
pub use gesture::{NewTab, StripDrag, TabDrop};
pub use pill::{MemberKind, MemberView, ShellLook, TabView};
pub use span::Span;
pub use tab_bar::{Clock, SidebarState, TabBar};
