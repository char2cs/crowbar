use layout::PaneInstance;
use protocol::{DocumentPath, PaneState, PaneTypeId, Params};
use serde_json::Value;

use super::arrival::Arrival;
use super::pane_view::PaneView;
use crate::core_panes::EDITOR;
use crate::tab_looks;

const THREAD_ID: &str = "threadId";
const PATH: &str = "path";
const ABSOLUTE: &str = "absolute";
const UNTITLED: &str = "untitled";
const FRESH: &str = "fresh";
const LINE: &str = "line";
const NAME: &str = "name";
const PREVIEW: &str = "preview";
const MARKDOWN_PREVIEW: &str = "markdown";
const HTML_PREVIEW: &str = "html";
const UNTITLED_SCHEME: &str = "untitled:";

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct EditorTarget {
    pub(crate) chat: String,
    pub(crate) file: Option<DocumentPath>,
    pub(crate) label: String,
    pub(crate) arrival: Arrival,
    pub(crate) line: Option<u32>,
    pub(crate) preview: Option<editor::Preview>,
}

impl EditorTarget {
    pub(crate) fn of(instance: &PaneInstance) -> Option<Self> {
        Self::of_pane((&instance.type_id, &instance.params, &instance.state))
    }

    pub(crate) fn of_pane(
        (type_id, params, state): (&PaneTypeId, &Params, &PaneState)
    ) -> Option<Self> {
        if type_id.as_str() != EDITOR {
            return None;
        }
        let chat = params.get(THREAD_ID)?.to_owned();
        let state = state.as_value();
        let arrival = match state.get(FRESH).and_then(Value::as_bool) {
            Some(true) => Arrival::Fresh,
            _ => Arrival::Restored,
        };
        let line = state
            .get(LINE)
            .and_then(Value::as_u64)
            .filter(|line| *line > 0);
        let line = line.and_then(|line| u32::try_from(line).ok());
        let file = match state.get(UNTITLED).and_then(Value::as_bool) {
            Some(true) => None,
            _ => Some(file_of(state)?),
        };
        let label = label_of(file.as_ref(), state);
        let preview = state
            .get(PREVIEW)
            .and_then(Value::as_str)
            .and_then(preview_of);
        Some(Self {
            chat,
            file,
            label,
            arrival,
            line,
            preview,
        })
    }

    pub(crate) fn view(&self) -> PaneView {
        if self.preview.is_some() {
            return PaneView::Preview;
        }
        let rich = self
            .file
            .as_ref()
            .is_some_and(|file| editor::Preview::has_rich_view(file.as_str()));
        if rich {
            return PaneView::Rich;
        }
        PaneView::Source
    }

    pub(crate) const fn preview_name(preview: editor::Preview) -> &'static str {
        match preview {
            editor::Preview::Markdown => MARKDOWN_PREVIEW,
            editor::Preview::Html => HTML_PREVIEW,
        }
    }

    pub(crate) fn untitled_label(name: &str) -> String {
        format!("{UNTITLED_SCHEME}{name}")
    }

    pub(crate) fn settled(state: &PaneState) -> Option<PaneState> {
        let value = state.as_value();
        let marked = value.get(FRESH).is_some() || value.get(LINE).is_some();
        if !marked {
            return None;
        }
        let mut kept = value.as_object()?.clone();
        kept.remove(FRESH);
        kept.remove(LINE);
        PaneState::new(Value::Object(kept)).ok()
    }
}

fn label_of(
    file: Option<&DocumentPath>,
    state: &Value,
) -> String {
    if let Some(file) = file {
        return file.as_str().to_owned();
    }
    let name = state
        .get(NAME)
        .and_then(Value::as_str)
        .unwrap_or(tab_looks::UNTITLED);
    EditorTarget::untitled_label(name)
}

fn preview_of(name: &str) -> Option<editor::Preview> {
    match name {
        MARKDOWN_PREVIEW => Some(editor::Preview::Markdown),
        HTML_PREVIEW => Some(editor::Preview::Html),
        _ => None,
    }
}

fn file_of(state: &Value) -> Option<DocumentPath> {
    let path = state.get(PATH).and_then(Value::as_str)?;
    let absolute = state.get(ABSOLUTE).and_then(Value::as_bool) == Some(true);
    let file = DocumentPath::new(path).ok()?;
    let host_mismatch = absolute != file.is_host();
    (!host_mismatch).then_some(file)
}

#[cfg(test)]
mod tests {
    use protocol::{PaneId, PaneTypeId, Params};
    use serde_json::json;

    use super::*;

    fn instance(state: Value) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new("p1").unwrap(),
            type_id: PaneTypeId::new(EDITOR).unwrap(),
            params: Params::default().with(THREAD_ID, "c1").unwrap(),
            state: PaneState::new(state).unwrap(),
        }
    }

    type Case = (Value, Option<(Option<&'static str>, Arrival, Option<u32>)>);

    fn cases() -> [Case; 5] {
        [
            (
                json!({"path": "src/a.ts", "fresh": true, "line": 4}),
                Some((Some("src/a.ts"), Arrival::Fresh, Some(4))),
            ),
            (
                json!({"path": "src/a.ts"}),
                Some((Some("src/a.ts"), Arrival::Restored, None)),
            ),
            (
                json!({"path": "/etc/hosts", "absolute": true}),
                Some((Some("/etc/hosts"), Arrival::Restored, None)),
            ),
            (
                json!({"untitled": true}),
                Some((None, Arrival::Restored, None)),
            ),
            (json!({}), None),
        ]
    }

    fn seen(target: EditorTarget) -> (Option<String>, Arrival, Option<u32>) {
        let file = target.file.map(|file| file.as_str().to_owned());
        (file, target.arrival, target.line)
    }

    #[test]
    fn a_file_pane_names_its_chat_file_and_how_it_arrived() {
        for (state, expected) in cases() {
            let seen = EditorTarget::of(&instance(state.clone())).map(seen);
            let expected =
                expected.map(|(file, arrival, line)| (file.map(str::to_owned), arrival, line));
            assert_eq!(seen, expected, "{state}");
        }
    }

    #[test]
    fn a_file_is_labelled_by_its_path_and_an_untitled_one_by_its_name() {
        let cases = [
            (json!({"path": "src/a.ts"}), "src/a.ts"),
            (
                json!({"untitled": true, "name": "Untitled-2"}),
                "untitled:Untitled-2",
            ),
            (json!({"untitled": true}), "untitled:Untitled"),
        ];
        for (state, expected) in cases {
            let target = EditorTarget::of(&instance(state.clone())).unwrap();
            assert_eq!(target.label, expected, "{state}");
        }
    }

    #[test]
    fn a_preview_pane_names_how_its_page_renders_the_file() {
        let cases = [
            (
                json!({"path": "README.md", "preview": "markdown"}),
                Some(editor::Preview::Markdown),
            ),
            (
                json!({"path": "a.html", "preview": "html"}),
                Some(editor::Preview::Html),
            ),
            (json!({"path": "README.md"}), None),
            (json!({"path": "README.md", "preview": "pdf"}), None),
        ];
        for (state, expected) in cases {
            let target = EditorTarget::of(&instance(state.clone())).unwrap();
            assert_eq!(target.preview, expected, "{state}");
            let named = expected.map(EditorTarget::preview_name);
            assert_eq!(
                named,
                state["preview"].as_str().filter(|_| expected.is_some()),
                "{state}"
            );
        }
    }

    #[test]
    fn a_shown_file_drops_its_open_marks_once() {
        let state = PaneState::new(json!({"path": "a.ts", "fresh": true, "line": 2})).unwrap();
        let settled = EditorTarget::settled(&state).unwrap();
        assert_eq!(settled.as_value(), &json!({"path": "a.ts"}));
        assert_eq!(
            EditorTarget::settled(&settled),
            None,
            "nothing left to drop"
        );
    }
}
