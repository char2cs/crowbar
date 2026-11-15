use protocol::{PaneTypeId, Params};
use shell_data::{ChatSurface, ChatTerminal};

use crate::core_panes::{CHAT, TERMINAL};

pub(crate) fn route(
    (type_id, params): (PaneTypeId, Params),
    chat: Option<ChatTerminal>,
) -> (PaneTypeId, Params) {
    let terminal = chat.filter(|chat| chat.surface == ChatSurface::Terminal);
    let routed = PaneTypeId::new(TERMINAL)
        .ok()
        .filter(|_| type_id.as_str() == CHAT)
        .filter(|_| terminal.is_some());
    (routed.unwrap_or(type_id), params)
}

#[cfg(test)]
mod tests {
    use shell_data::ChatPhase;

    use super::*;
    use crate::core_panes::{chat_params, chat_type};

    const CASES: [(Option<ChatSurface>, &str); 3] = [
        (None, CHAT),
        (Some(ChatSurface::Provider), CHAT),
        (Some(ChatSurface::Terminal), TERMINAL),
    ];

    fn dormant(surface: ChatSurface) -> ChatTerminal {
        ChatTerminal {
            surface,
            live_runner_id: None,
            terminal_session_id: None,
            phase: ChatPhase::Dormant,
        }
    }

    #[test]
    fn only_a_terminal_surface_chat_opens_in_a_terminal_pane() {
        for (surface, expected) in CASES {
            let chat = surface.map(dormant);
            let opened = (chat_type().unwrap(), chat_params("c1").unwrap());
            let (type_id, params) = route(opened, chat);
            assert_eq!(type_id.as_str(), expected);
            assert_eq!(params.get("threadId"), Some("c1"));
        }
    }
}
