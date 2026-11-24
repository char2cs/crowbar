//! Go to definition from the document's language server, as develop's Monaco definition
//! provider asks it: the daemon answers workspace-relative locations, and absolute ones lie
//! outside the workspace.

use std::rc::Rc;

use gpui::{App, AppContext as _, Task, Window};
use gpui_kit::base::input::{DefinitionProvider, Rope};
use lsp_types::{LocationLink, Range, Uri};
use serde_json::{Map, Value, json};

use crate::EditorEvent;
use crate::lsp_link::LspLink;
use crate::lsp_position::{range_from_lsp, to_lsp};

const WORKSPACE_SCHEME: &str = "crowbar-workspace";
const OUTSIDE_SCHEME: &str = "file";
const OUTSIDE_ROOT: char = '/';

pub(crate) struct LspDefinition {
    pub(crate) link: Rc<LspLink>,
}

impl DefinitionProvider for LspDefinition {
    fn definitions(
        &self,
        text: &Rope,
        offset: usize,
        _window: &mut Window,
        cx: &mut App,
    ) -> Task<gpui::Result<Vec<LocationLink>>> {
        let mut fields = Map::new();
        fields.insert("position".to_owned(), json!(to_lsp(text, offset)));
        let asked = self.link.request("definition", fields, cx);
        let own = self.link.path();
        let text = text.clone();
        cx.background_spawn(async move {
            let answer = asked.await?;
            let chosen = chosen(locations_of(&answer));
            let linked = chosen
                .into_iter()
                .map(|found| own_in_chars(found, (own.as_deref(), &text)));
            Ok(linked.filter_map(link_of).collect())
        })
    }
}

pub(crate) fn elsewhere(
    shown: &lsp_types::ShowDocumentParams,
    own: Option<&str>,
) -> Option<EditorEvent> {
    let path = shown.uri.path().as_str();
    let relative = path.trim_start_matches(OUTSIDE_ROOT);
    let line = shown.selection.map_or(0, |range| range.start.line);
    match shown.uri.scheme()?.as_str() {
        WORKSPACE_SCHEME if Some(relative) != own => Some(EditorEvent::Definition {
            path: relative.to_owned(),
            line,
        }),
        OUTSIDE_SCHEME => Some(EditorEvent::DefinitionOutside(path.to_owned())),
        _ => None,
    }
}

fn locations_of(answer: &Value) -> Vec<(String, Range)> {
    let Some(found) = answer.as_array() else {
        return Vec::new();
    };
    found.iter().filter_map(location_of).collect()
}

fn location_of(found: &Value) -> Option<(String, Range)> {
    let path = found.get("filePath")?.as_str()?.to_owned();
    let range = serde_json::from_value(found.get("range")?.clone()).ok()?;
    Some((path, range))
}

fn chosen(found: Vec<(String, Range)>) -> Vec<(String, Range)> {
    let inside: Vec<(String, Range)> = found
        .iter()
        .filter(|(path, _)| !is_outside(path))
        .cloned()
        .collect();
    if inside.is_empty() {
        return found;
    }
    inside
}

fn own_in_chars(
    (path, range): (String, Range),
    (own, text): (Option<&str>, &Rope),
) -> (String, Range) {
    if Some(path.as_str()) != own {
        return (path, range);
    }
    (path, range_from_lsp(text, range))
}

fn is_outside(path: &str) -> bool {
    path.starts_with(OUTSIDE_ROOT)
}

fn link_of((path, range): (String, Range)) -> Option<LocationLink> {
    let uri = uri_of(&path).parse::<Uri>().ok()?;
    Some(LocationLink {
        origin_selection_range: None,
        target_uri: uri,
        target_range: range,
        target_selection_range: range,
    })
}

fn uri_of(path: &str) -> String {
    if is_outside(path) {
        return format!("{OUTSIDE_SCHEME}://{path}");
    }
    format!("{WORKSPACE_SCHEME}:///{path}")
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use serde_json::json;

    use super::*;
    use crate::editor_view::test_support::{listened, selected_of, served};

    fn at(
        path: &str,
        (line, from, to): (u32, u32, u32),
    ) -> Value {
        let range = json!({"start": {"line": line, "character": from}, "end": {"line": line, "character": to}});
        json!({"filePath": path, "range": range})
    }

    fn told_after_f12(
        answer: Value,
        cx: &mut TestAppContext,
    ) -> Vec<EditorEvent> {
        let (view, window, server) = served(("src/a.ts", "let a = b"), cx);
        server.answer("definition", answer);
        let heard = listened(&view, window);
        window.simulate_keystrokes("f12");
        window.run_until_parked();
        heard.take()
    }

    #[gpui::test]
    fn a_definition_in_another_file_is_told_with_its_line(cx: &mut TestAppContext) {
        let answer = json!([at("/usr/lib/x.d.ts", (0, 0, 1)), at("src/b.ts", (3, 2, 3))]);
        let told = told_after_f12(answer, cx);
        let definition = EditorEvent::Definition {
            path: "src/b.ts".to_owned(),
            line: 3,
        };
        assert_eq!(told, [definition]);
    }

    #[gpui::test]
    fn a_definition_only_outside_the_workspace_is_told_as_outside(cx: &mut TestAppContext) {
        let told = told_after_f12(json!([at("/usr/lib/x.d.ts", (0, 0, 1))]), cx);
        let outside = EditorEvent::DefinitionOutside("/usr/lib/x.d.ts".to_owned());
        assert_eq!(told, [outside]);
    }

    #[gpui::test]
    fn a_definition_in_the_same_file_past_an_emoji_selects_it_in_place(cx: &mut TestAppContext) {
        let (view, window, server) = served(("src/a.ts", "\"😀\"; let b = 1"), cx);
        server.answer("definition", json!([at("src/a.ts", (0, 10, 11))]));
        let heard = listened(&view, window);
        window.simulate_keystrokes("f12");
        window.run_until_parked();
        assert_eq!(heard.take(), []);
        assert_eq!(selected_of(&view, window), 12..13);
    }
}
