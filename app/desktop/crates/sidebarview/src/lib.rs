//! The native sidebar of the Crowbar shell: the [`Sidebar`] gpui entity, which frames the content
//! view and draws the header, space switcher, mode bar, the Chats, Files and Changes panels, menus,
//! dialogs, peek card and toasts. Its data and rules live in the gpui-free `sidebar` crate.

mod column;
mod overlays;
mod panels;
mod sidebar;
#[cfg(test)]
mod test_support;

pub use panels::{ChatGlyph, FileGlyph};
pub use sidebar::{Event, Sidebar};
