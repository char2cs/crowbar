//! The Crowbar editor's documents without GPUI: the language a path is written in, as develop's
//! `language-id.ts` resolves it, why a document could not be opened, and the drafts that keep a
//! dirty document's text across restarts, and the text commands the editor runs over a
//! document: occurrences of a word, word lookup and whole-line edits, and how its text is
//! marked: whitespace and highlight query rewrites.

mod drafts;
mod editor;
mod error;
mod find_query;
mod goto_target;
pub(crate) mod language;
mod line_edit;
mod open_failure;
mod whitespace_mode;

pub use drafts::draft::Draft;
pub use drafts::{DiskDraftFile, DraftFile, Drafts};
pub use editor::{host_uri, workspace_uri};
pub use error::Error;
pub use find_query::FindQuery;
pub use find_query::find_option::FindOption;
pub use find_query::find_options::FindOptions;
pub use find_query::word::word_at;
pub use goto_target::GotoTarget;
pub use language::Language;
pub use language::grammar;
pub use language::preview::Preview;
pub use line_edit::LineEdit;
pub use line_edit::vertical::Vertical;
pub use open_failure::OpenFailure;
pub use whitespace_mode::{WhitespaceMode, marked_whitespace};
