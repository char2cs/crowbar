use pane_host::{Multiplicity, PaneKind, PaneLifecycle, PaneRegistry, PaneType};
use protocol::{PaneTypeId, Params};

use crate::core_type::CoreType;
use crate::error::Result;

pub(crate) const CHAT: &str = "core.chat";
pub(crate) const EDITOR: &str = "core.editor";
pub(crate) const REVIEW: &str = "core.review";
pub(crate) const CONSOLE: &str = "core.console";
pub(crate) const TERMINAL: &str = "core.terminal";
pub(crate) const NEW_TAB: &str = "core.newtab";
pub(crate) const BROWSER: &str = browser_nav::BROWSER_PANE_TYPE;

pub(crate) fn chat_type() -> Result<PaneTypeId> {
    Ok(PaneTypeId::new(CHAT)?)
}

pub(crate) fn chat_params(thread: &str) -> Result<Params> {
    Ok(Params::default().with("threadId", thread)?)
}

const TYPES: [CoreType; 6] = [
    CoreType {
        id: EDITOR,
        title: "File",
        entry: "editor/index.html",
        kind: PaneKind::Web,
        lifecycle: PaneLifecycle::Parkable,
        multiplicity: Multiplicity::PerParams,
    },
    CoreType {
        id: REVIEW,
        title: "Review",
        entry: "review/index.html",
        kind: PaneKind::Web,
        lifecycle: PaneLifecycle::Parkable,
        multiplicity: Multiplicity::PerParams,
    },
    CoreType {
        id: CONSOLE,
        title: "Console",
        entry: "console/index.html",
        kind: PaneKind::Web,
        lifecycle: PaneLifecycle::KeepAlive,
        multiplicity: Multiplicity::Single,
    },
    CoreType {
        id: TERMINAL,
        title: "Terminal",
        entry: "",
        kind: PaneKind::Native,
        lifecycle: PaneLifecycle::KeepAlive,
        multiplicity: Multiplicity::Many,
    },
    CoreType {
        id: NEW_TAB,
        title: "New Tab",
        entry: "",
        kind: PaneKind::Native,
        lifecycle: PaneLifecycle::KeepAlive,
        multiplicity: Multiplicity::Many,
    },
    CoreType {
        id: BROWSER,
        title: "Browser",
        entry: "",
        kind: PaneKind::Web,
        lifecycle: PaneLifecycle::KeepAlive,
        multiplicity: Multiplicity::Many,
    },
];

pub(crate) fn register_core_panes(
    registry: &mut PaneRegistry,
    chat_entry: &str,
) -> Result<()> {
    registry.register(PaneType {
        id: chat_type()?,
        title: "Chat".into(),
        entry: chat_entry.into(),
        kind: PaneKind::Web,
        lifecycle: PaneLifecycle::Parkable,
        multiplicity: Multiplicity::PerParams,
    })?;
    for spec in TYPES {
        registry.register(pane_type(&spec)?)?;
    }
    Ok(())
}

fn pane_type(spec: &CoreType) -> Result<PaneType> {
    Ok(PaneType {
        id: PaneTypeId::new(spec.id)?,
        title: spec.title.into(),
        entry: spec.entry.into(),
        kind: spec.kind,
        lifecycle: spec.lifecycle,
        multiplicity: spec.multiplicity,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn registered() -> PaneRegistry {
        let mut registry = PaneRegistry::default();
        register_core_panes(&mut registry, "chat/index.html").unwrap();
        registry
    }

    const CASES: [(&str, bool); 7] = [
        (CHAT, true),
        (EDITOR, true),
        (REVIEW, true),
        (CONSOLE, true),
        (TERMINAL, false),
        (NEW_TAB, false),
        (BROWSER, false),
    ];

    #[test]
    fn every_core_type_is_registered_and_only_bundled_web_ones_have_an_entry() {
        let registry = registered();
        for (id, bundled) in CASES {
            let pane_type = registry.get(&PaneTypeId::new(id).unwrap()).unwrap();
            assert_eq!(!pane_type.entry.is_empty(), bundled, "{id}");
        }
    }

    #[test]
    fn a_second_registration_is_refused() {
        let mut registry = registered();
        assert!(register_core_panes(&mut registry, "chat/index.html").is_err());
    }

    #[test]
    fn a_chat_is_identified_by_its_thread() {
        let params = chat_params("t1").unwrap();
        assert_eq!(params.get("threadId"), Some("t1"), "thread id");
    }
}
