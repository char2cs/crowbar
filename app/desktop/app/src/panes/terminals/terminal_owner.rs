use protocol::Params;
use ptysession::SessionPaths;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum TerminalOwner {
    Chat(String),
    Home(String),
}

impl TerminalOwner {
    pub(crate) fn of(params: &Params) -> Option<Self> {
        let chat = params
            .get("threadId")
            .map(|chat| Self::Chat(chat.to_owned()));
        chat.or_else(|| params.get("projectId").map(|id| Self::Home(id.to_owned())))
    }

    pub(crate) fn base(&self) -> ptysession::Result<SessionPaths> {
        match self {
            Self::Chat(chat) => SessionPaths::chat(chat),
            Self::Home(project) => SessionPaths::project_home(project),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, &str, &str); 3] = [
        ("threadId", "c1", "/v0/chats/c1/terminals"),
        ("projectId", "p1", "/v0/projects/p1/home/terminals"),
        ("other", "x", ""),
    ];

    #[test]
    fn a_chat_or_a_project_home_owns_the_terminal() {
        for (key, id, route) in CASES {
            let params = Params::default().with(key, id).unwrap();
            let base = TerminalOwner::of(&params).map(|owner| owner.base().unwrap());
            let path = base.as_ref().map_or("", SessionPaths::create_path);
            assert_eq!(path, route, "{key}");
        }
    }

    #[test]
    fn a_home_terminal_spec_names_its_project() {
        let spec = crate::panes::kinds::pane_spec::PaneSpec::home_terminal("t1", "p1").unwrap();
        assert_eq!(
            TerminalOwner::of(&spec.params),
            Some(TerminalOwner::Home("p1".into()))
        );
    }
}
