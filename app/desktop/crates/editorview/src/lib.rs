//! GPUI view of one editor pane: the gpui-kit `Editor` over a document's text.

mod chrome;
mod document;
mod editor_event;
mod editor_view;
mod editorview;
mod keys;
mod lsp;
mod opening;
mod settings;
mod theme;

pub use chrome::toolbar::caret::Caret;
pub use document::Document;
pub use document::disk_take::DiskTake;
pub use document::document_event::DocumentEvent;
pub use document::document_opening::DocumentOpening;
pub use document::save_mark::SaveMark;
pub use editor_event::EditorEvent;
pub use editor_view::EditorView;
pub use editorview::init;
pub use lsp::language_server::LanguageServer;
pub use lsp::lsp_error::LspError;
pub use opening::Opening;
pub use opening::access::Access;
pub use settings::Settings;
pub use settings::word_wrap::WordWrap;
