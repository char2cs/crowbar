//! The Crowbar editor's documents without GPUI: the language a path is written in, as develop's
//! `language-id.ts` resolves it, why a document could not be opened, and the drafts that keep a
//! dirty document's text across restarts.

mod draft;
mod drafts;
mod editor;
mod language;
mod open_failure;

pub use draft::Draft;
pub use drafts::{DiskDraftFile, DraftFile, Drafts};
pub use editor::{host_uri, workspace_uri};
pub use language::Language;
pub use open_failure::OpenFailure;
