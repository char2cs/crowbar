//! A document at its language server: the link, positions, completion, definition, hover,
//! semantic tokens and formatting.

pub(crate) mod language_server;
pub(crate) mod lsp_completion;
pub(crate) mod lsp_definition;
pub(crate) mod lsp_error;
pub(crate) mod lsp_formatting;
pub(crate) mod lsp_hover;
pub(crate) mod lsp_link;
pub(crate) mod lsp_position;
pub(crate) mod lsp_semantic_tokens;
