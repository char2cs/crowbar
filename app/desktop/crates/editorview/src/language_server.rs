use gpui::{App, Task};
use serde_json::Value;

use crate::LspError;

/// The daemon's language servers for the chat whose pane shows a view, as develop's `LspClient`
/// reaches them: one LSP route per verb, addressed by the shell.
pub trait LanguageServer {
    /// Posts `body` to the LSP route `verb` (`didOpen`, `hover` …) and resolves to the data the
    /// daemon answered: `null` where no server serves the document.
    fn post(
        &self,
        verb: &'static str,
        body: Value,
        cx: &mut App,
    ) -> Task<Result<Value, LspError>>;
}
