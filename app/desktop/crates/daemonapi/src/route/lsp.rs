//! The routes of the language servers a chat owns.

use crate::route::Route;
use crate::route::query::encode;

impl Route {
    /// The LSP route `verb` (`hover`, `didOpen` …) of the language servers `chat` owns.
    pub fn lsp(
        chat: &str,
        verb: &str,
    ) -> Self {
        Self::at(format!("/v0/chats/{chat}/lsp/{verb}"))
    }

    /// The route of the state of the language server `chat` runs for the file at `path`,
    /// percent-encoded.
    pub fn lsp_status(
        chat: &str,
        path: &str,
    ) -> Self {
        Self::at(format!(
            "{}?path={}",
            Self::lsp(chat, "status"),
            encode(path)
        ))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_chat_s_lsp_verbs_nest_under_it_and_the_status_names_its_file() {
        let cases = [
            (Route::lsp("c", "hover"), "/v0/chats/c/lsp/hover"),
            (
                Route::lsp_status("c", "src/a b.ts"),
                "/v0/chats/c/lsp/status?path=src/a%20b.ts",
            ),
        ];
        for (route, path) in cases {
            assert_eq!(String::from(route), path);
        }
    }
}
