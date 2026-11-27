use crate::route::Route;
use crate::route::query::encode;

impl Route {
    pub fn lsp(
        chat: &str,
        verb: &str,
    ) -> Self {
        Self::at(format!("/v0/chats/{chat}/lsp/{verb}"))
    }

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
