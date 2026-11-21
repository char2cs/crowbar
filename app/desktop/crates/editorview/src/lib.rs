//! GPUI view of one editor pane: the gpui-kit `Editor` over a document's text.

mod access;
mod caret;
mod editor_event;
mod editor_view;
mod editorview;
mod highlight_name;
mod opening;
mod save_key;
mod toolbar;

pub use access::Access;
pub use caret::Caret;
pub use editor_event::EditorEvent;
pub use editor_view::EditorView;
pub use editorview::init;
pub use opening::Opening;
