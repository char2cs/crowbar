use store::{ChatSurface, ChatTerminal};

#[derive(Debug, PartialEq, Eq)]
pub(crate) enum Plan {
    Own,
    Watch(String),
    Resume,
}

impl Plan {
    pub(crate) fn of(state: Option<&ChatTerminal>) -> Self {
        let Some(chat) = state.filter(|chat| chat.surface == ChatSurface::Terminal) else {
            return Self::Own;
        };
        chat.terminal_session_id
            .clone()
            .map_or(Self::Resume, Self::Watch)
    }
}

#[cfg(test)]
mod tests {
    use store::ChatPhase;

    use super::*;

    fn chat(
        surface: ChatSurface,
        session: Option<&str>,
    ) -> ChatTerminal {
        ChatTerminal {
            surface,
            live_runner_id: session.map(|_| "r".to_owned()),
            terminal_session_id: session.map(str::to_owned),
            phase: ChatPhase::Live,
        }
    }

    #[test]
    fn a_terminal_surface_chat_is_watched_or_resumed_never_created() {
        let cases = [
            (None, Plan::Own),
            (Some(chat(ChatSurface::Provider, Some("s1"))), Plan::Own),
            (
                Some(chat(ChatSurface::Terminal, Some("s1"))),
                Plan::Watch("s1".into()),
            ),
            (Some(chat(ChatSurface::Terminal, None)), Plan::Resume),
        ];
        for (state, expected) in cases {
            assert_eq!(Plan::of(state.as_ref()), expected);
        }
    }
}
