use protocol::{PaneState, Params};
use serde_json::json;

use super::{PaneSpec, THREAD_ID};
use crate::core_panes::EDITOR;
use crate::editor_panes::EditorTarget;
use crate::error::Result;

const FNV_OFFSET_BASIS: u64 = 0xcbf2_9ce4_8422_2325;
const FNV_PRIME: u64 = 0x0100_0000_01b3;
const FILE_ID: &str = "fileId";
const PATH: &str = "path";
const VIEW: &str = "view";
const PREVIEW_VIEW: &str = "preview";

fn file_id(path: &str) -> String {
    let hash = path.bytes().fold(FNV_OFFSET_BASIS, |hash, byte| {
        (hash ^ u64::from(byte)).wrapping_mul(FNV_PRIME)
    });
    format!("f{hash:016x}")
}

impl PaneSpec {
    pub(crate) fn editor(
        chat: &str,
        path: &str,
    ) -> Result<Self> {
        let params = Params::default()
            .with(THREAD_ID, chat)?
            .with(FILE_ID, file_id(path))?;
        Self::new(
            EDITOR,
            params,
            PaneState::new(json!({ PATH: path, "fresh": true }))?,
        )
    }

    pub(crate) fn untitled(
        chat: &str,
        (id, name): (&str, &str),
    ) -> Result<Self> {
        let params = Params::default()
            .with(THREAD_ID, chat)?
            .with(FILE_ID, format!("u{id}"))?;
        let state = PaneState::new(json!({ "untitled": true, "name": name }))?;
        Self::new(EDITOR, params, state)
    }

    pub(crate) fn preview(
        chat: &str,
        (file, preview): (&protocol::DocumentPath, editor::Preview),
    ) -> Result<Self> {
        let path = file.as_str();
        let params = Params::default()
            .with(THREAD_ID, chat)?
            .with(FILE_ID, file_id(path))?
            .with(VIEW, PREVIEW_VIEW)?;
        let preview = EditorTarget::preview_name(preview);
        let state = json!({ PATH: path, "absolute": file.is_host(), "preview": preview });
        Self::new(EDITOR, params, PaneState::new(state)?)
    }

    pub(crate) fn host_file(
        chat: &str,
        path: &str,
    ) -> Result<Self> {
        let params = Params::default()
            .with(THREAD_ID, chat)?
            .with(FILE_ID, file_id(path))?;
        let state = PaneState::new(json!({ PATH: path, "absolute": true, "fresh": true }))?;
        Self::new(EDITOR, params, state)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (
        &'static str,
        fn() -> Result<PaneSpec>,
        &'static str,
        &'static str,
    );

    const CASES: [Case; 3] = [
        (
            "editor",
            || PaneSpec::editor("c1", "src/main.rs"),
            "path",
            "\"src/main.rs\"",
        ),
        (
            "untitled",
            || PaneSpec::untitled("c1", ("p4", "Untitled-1")),
            "untitled",
            "true",
        ),
        (
            "host file",
            || PaneSpec::host_file("c1", "/home/a.rs"),
            "absolute",
            "true",
        ),
    ];

    #[test]
    fn a_file_spec_carries_what_it_needs_in_its_state() {
        for (name, spec, key, expected) in CASES {
            let state = spec().unwrap().state;
            assert_eq!(state.as_value()[key].to_string(), expected, "{name}");
        }
    }

    #[test]
    fn a_file_opened_now_is_marked_fresh_so_a_refused_read_closes_it() {
        let specs = [
            PaneSpec::editor("c1", "src/main.rs"),
            PaneSpec::host_file("c1", "/home/a.rs"),
        ];
        for spec in specs {
            assert_eq!(spec.unwrap().state.as_value()["fresh"], true);
        }
    }

    #[test]
    fn a_file_is_identified_by_its_path_without_a_slash_in_the_params() {
        let a = PaneSpec::editor("c1", "src/main.rs").unwrap();
        let again = PaneSpec::editor("c1", "src/main.rs").unwrap();
        let other = PaneSpec::editor("c1", "src/lib.rs").unwrap();
        assert_eq!(a.params, again.params, "same file, same pane");
        assert_ne!(a.params, other.params, "other file, other pane");
    }

    #[test]
    fn untitled_and_picked_files_are_told_apart_from_workspace_files() {
        let untitled = PaneSpec::untitled("c1", ("p4", "Untitled-1")).unwrap();
        let picked = PaneSpec::host_file("c1", "/home/a.rs").unwrap();
        assert_ne!(untitled.params, picked.params, "ids");
    }

    #[test]
    fn a_very_long_path_still_fits_the_params() {
        let path = "a/".repeat(2000);
        assert!(PaneSpec::editor("c1", &path).is_ok());
    }

    #[test]
    fn a_preview_is_a_pane_of_its_own_beside_its_file_s_editor() {
        let file = protocol::DocumentPath::new("README.md").unwrap();
        let preview = PaneSpec::preview("c1", (&file, editor::Preview::Markdown)).unwrap();
        let editor = PaneSpec::editor("c1", "README.md").unwrap();
        assert_ne!(preview.params, editor.params, "two panes");
        assert_eq!(
            preview.state.as_value(),
            &json!({"path": "README.md", "absolute": false, "preview": "markdown"})
        );
        let host = protocol::DocumentPath::new("/home/a.html").unwrap();
        let host = PaneSpec::preview("c1", (&host, editor::Preview::Html)).unwrap();
        assert_eq!(host.state.as_value()["absolute"], true);
    }

    #[test]
    fn file_ids_are_stable() {
        assert_eq!(file_id("a"), "faf63dc4c8601ec8c");
    }
}
