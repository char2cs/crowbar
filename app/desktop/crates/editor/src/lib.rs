//! The Crowbar editor's documents without GPUI: the language a path is written in, as develop's
//! `language-id.ts` resolves it, and why a document could not be opened.

mod language;
mod open_failure;

pub use language::Language;
pub use open_failure::OpenFailure;
