use ideshell::{TabAccess, TabKeep, TabLook};
use layout::PaneInstance;
use ui::IconName;

use crate::chat_label::NEW_CHAT;
use crate::core_panes::{BROWSER, CHAT, CONSOLE, EDITOR, NEW_TAB, REVIEW, TERMINAL};

pub(crate) const UNTITLED: &str = "Untitled";
const THREAD_ID: &str = "threadId";

pub(crate) fn tab_look(
    instance: &PaneInstance,
    title: Option<&str>,
) -> TabLook {
    let (fallback, icon) = by_type(instance);
    let label = title.filter(|text| !text.is_empty()).unwrap_or(&fallback);
    let look = TabLook {
        label: label.to_owned().into(),
        icon,
        artwork: None,
        path: None,
        dirty: false,
        renamable: instance.type_id.as_str() == TERMINAL,
        keep: TabKeep::Kept,
        access: TabAccess::Writable,
    };
    match file_path(instance) {
        Some(path) => look.at_path(path),
        None => look,
    }
}

pub(crate) fn chat_of(instance: &PaneInstance) -> Option<&str> {
    if instance.type_id.as_str() != CHAT {
        return None;
    }
    instance.params.get(THREAD_ID)
}

pub(crate) fn file_chat_of(instance: &PaneInstance) -> Option<&str> {
    if instance.type_id.as_str() != EDITOR {
        return None;
    }
    instance.params.get(THREAD_ID)
}

pub(crate) fn is_file(instance: &PaneInstance) -> bool {
    instance.type_id.as_str() == EDITOR
}

fn file_path(instance: &PaneInstance) -> Option<String> {
    if instance.type_id.as_str() != EDITOR {
        return None;
    }
    let state = instance.state.as_value();
    let path = state.get("path").and_then(serde_json::Value::as_str);
    path.filter(|path| !path.is_empty()).map(str::to_owned)
}

fn by_type(instance: &PaneInstance) -> (String, IconName) {
    match instance.type_id.as_str() {
        CHAT => (NEW_CHAT.into(), IconName::ChatsCircle),
        TERMINAL => ("Terminal".into(), IconName::TerminalWindow),
        BROWSER => ("Browser".into(), IconName::Globe),
        REVIEW => ("Branch Review".into(), IconName::GitPullRequest),
        NEW_TAB => ("New Tab".into(), IconName::Plus),
        CONSOLE => ("Console".into(), IconName::TerminalWindow),
        EDITOR => (file_name(instance), IconName::FileText),
        other => (other.to_owned(), IconName::FileText),
    }
}

fn file_name(instance: &PaneInstance) -> String {
    let state = instance.state.as_value();
    let path = state.get("path").and_then(serde_json::Value::as_str);
    path.and_then(|path| path.rsplit('/').next())
        .filter(|name| !name.is_empty())
        .map_or_else(|| UNTITLED.to_owned(), str::to_owned)
}

#[cfg(test)]
mod tests {
    use protocol::{PaneId, PaneState, PaneTypeId, Params};

    use super::*;

    fn pane(
        kind: &str,
        state: serde_json::Value,
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new("p1").unwrap(),
            type_id: PaneTypeId::new(kind).unwrap(),
            params: Params::default(),
            state: PaneState::new(state).unwrap(),
        }
    }

    struct Case {
        name: &'static str,
        kind: &'static str,
        state: &'static str,
        title: Option<&'static str>,
        label: &'static str,
        icon: IconName,
    }

    const FILE_ICON: IconName = IconName::FileText;

    const CASES: [Case; 8] = [
        Case {
            name: "an untitled chat",
            kind: CHAT,
            state: "null",
            title: None,
            label: NEW_CHAT,
            icon: IconName::ChatsCircle,
        },
        Case {
            name: "a titled chat",
            kind: CHAT,
            state: "null",
            title: Some("Fix the build"),
            label: "Fix the build",
            icon: IconName::ChatsCircle,
        },
        Case {
            name: "a terminal without a title",
            kind: TERMINAL,
            state: "null",
            title: None,
            label: "Terminal",
            icon: IconName::TerminalWindow,
        },
        Case {
            name: "a reported title wins",
            kind: TERMINAL,
            state: "null",
            title: Some("cargo test"),
            label: "cargo test",
            icon: IconName::TerminalWindow,
        },
        Case {
            name: "an empty title is ignored",
            kind: BROWSER,
            state: "null",
            title: Some(""),
            label: "Browser",
            icon: IconName::Globe,
        },
        Case {
            name: "a file shows its name",
            kind: EDITOR,
            state: r#"{"path": "src/lib/main.rs"}"#,
            title: None,
            label: "main.rs",
            icon: FILE_ICON,
        },
        Case {
            name: "an untitled file",
            kind: EDITOR,
            state: r#"{"untitled": true}"#,
            title: None,
            label: "Untitled",
            icon: FILE_ICON,
        },
        Case {
            name: "the review",
            kind: REVIEW,
            state: "null",
            title: None,
            label: "Branch Review",
            icon: IconName::GitPullRequest,
        },
    ];

    #[test]
    fn a_tab_is_named_by_its_title_then_by_what_it_is() {
        for case in &CASES {
            let state = serde_json::from_str(case.state).unwrap();
            let look = tab_look(&pane(case.kind, state), case.title);
            assert_eq!(look.label.as_ref(), case.label, "{}", case.name);
            assert_eq!(look.icon, case.icon, "{}: icon", case.name);
        }
    }

    #[test]
    fn only_a_terminal_tab_can_be_renamed() {
        for (kind, expected) in [(TERMINAL, true), (EDITOR, false), (BROWSER, false)] {
            let look = tab_look(&pane(kind, serde_json::Value::Null), None);
            assert_eq!(look.renamable, expected, "{kind}");
        }
    }

    #[test]
    fn only_a_saved_file_names_its_path_for_the_tab_menu() {
        let cases = [
            (
                "a file",
                EDITOR,
                r#"{"path": "src/main.rs"}"#,
                Some("src/main.rs"),
            ),
            ("an untitled file", EDITOR, r#"{"untitled": true}"#, None),
            ("a terminal", TERMINAL, r#"{"path": "x"}"#, None),
        ];
        for (name, kind, state, expected) in cases {
            let state = serde_json::from_str(state).unwrap();
            let look = tab_look(&pane(kind, state), None);
            assert_eq!(look.path.as_deref(), expected, "{name}");
        }
    }
}
