use std::ops::Range;
use std::rc::Rc;

use gpui::{App, Task, Window};
use gpui_kit::base::input::{DocumentRangeSemanticTokensProvider, Rope};
use lsp_types::{SemanticTokenType, SemanticTokens, SemanticTokensLegend};
use serde_json::Map;

use crate::Highlighting;
use crate::Settings;
use crate::lsp::lsp_link::{CHANGE_DEBOUNCE, LspLink};

const VARIABLE: &str = "text.code.span";

const TYPE: &str = "type";
const FUNCTION: &str = "function";
const CLASSES: [&str; 26] = [
    TYPE,
    TYPE,
    TYPE,
    TYPE,
    TYPE,
    TYPE,
    TYPE,
    VARIABLE,
    VARIABLE,
    "property",
    "constant",
    "property",
    FUNCTION,
    FUNCTION,
    FUNCTION,
    "keyword",
    "keyword",
    "comment",
    "string",
    "number",
    "string.regex",
    "operator",
    "attribute",
    "constant",
    "tag",
    "attribute",
];
const VARIABLE_TYPE: u32 = 8;
const CONSTANT_TYPE: u32 = 10;
const READONLY: u32 = 1;

pub(crate) struct LspSemanticTokens {
    pub(crate) link: Rc<LspLink>,
}

impl DocumentRangeSemanticTokensProvider for LspSemanticTokens {
    fn legend(&self) -> SemanticTokensLegend {
        SemanticTokensLegend {
            token_types: CLASSES.map(SemanticTokenType::new).to_vec(),
            token_modifiers: Vec::new(),
        }
    }

    fn semantic_tokens(
        &self,
        _text: &Rope,
        _range: Range<usize>,
        _window: &mut Window,
        cx: &mut App,
    ) -> Task<gpui::Result<SemanticTokens>> {
        if Settings::of(cx).assists.highlighting == Highlighting::Syntax {
            return Task::ready(Ok(SemanticTokens::default()));
        }
        let paused = pause(&self.link, cx);
        let link = Rc::clone(&self.link);
        cx.spawn(async move |cx| {
            paused.await;
            let asked = cx.update(|cx| link.request("semanticTokens", Map::new(), cx));
            let answer: Option<SemanticTokens> = serde_json::from_value(asked.await?)?;
            Ok(readonly_as_constants(answer.unwrap_or_default()))
        })
    }
}

fn pause(
    link: &LspLink,
    cx: &App,
) -> Task<()> {
    if link.is_behind() {
        return cx.background_executor().timer(CHANGE_DEBOUNCE);
    }
    Task::ready(())
}

fn readonly_as_constants(mut tokens: SemanticTokens) -> SemanticTokens {
    let readonly = |token: &&mut lsp_types::SemanticToken| {
        token.token_type == VARIABLE_TYPE && token.token_modifiers_bitset & READONLY != 0
    };
    for token in tokens.data.iter_mut().filter(readonly) {
        token.token_type = CONSTANT_TYPE;
    }
    tokens
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_legend_follows_the_daemon_s_canonical_order() {
        const PINNED: [(usize, &str); 5] = [
            (0, TYPE),
            (VARIABLE_TYPE as usize, VARIABLE),
            (CONSTANT_TYPE as usize, "constant"),
            (15, "keyword"),
            (25, "attribute"),
        ];
        for (index, class) in PINNED {
            assert_eq!(CLASSES[index], class, "type {index}");
        }
    }
}
