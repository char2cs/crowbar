use layout_view::TabLook;
use model::PaneInstance;
use ui::IconName;

use crate::core_panes::{BROWSER, CONSOLE, EDITOR, NEW_TAB, REVIEW, TERMINAL};

const UNTITLED: &str = "Untitled";

pub(crate) fn tab_look(
    instance: &PaneInstance,
    title: Option<&str>,
) -> TabLook {
    let (fallback, icon) = by_type(instance);
    let label = title.filter(|text| !text.is_empty()).unwrap_or(&fallback);
    TabLook {
        label: label.to_owned().into(),
        icon,
    }
}

fn by_type(instance: &PaneInstance) -> (String, IconName) {
    match instance.type_id.as_str() {
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

    const CASES: [Case; 6] = [
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
}
