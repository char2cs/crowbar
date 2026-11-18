//! What a terminal-surface chat needs from the daemon's chat record.

use crate::chat_phase::ChatPhase;
use crate::chat_surface::ChatSurface;
use crate::state::State;

/// A chat's surface, runner and terminal session.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ChatTerminal {
    /// The face the chat is driven through.
    pub surface: ChatSurface,
    /// The live runner; `None` when dormant.
    pub live_runner_id: Option<String>,
    /// The runner's PTY session; `None` exactly when there is no runner.
    pub terminal_session_id: Option<String>,
    /// Where the runner is in its life.
    pub phase: ChatPhase,
}

fn present(text: &str) -> Option<String> {
    Some(text)
        .filter(|text| !text.is_empty())
        .map(str::to_owned)
}

impl ChatTerminal {
    pub(crate) fn of(
        state: &State,
        chat: &str,
    ) -> Option<Self> {
        let record = state
            .hosts
            .iter()
            .find_map(|slot| slot.store.chats.get(chat))?;
        Some(Self {
            surface: ChatSurface::from_wire(&record.surface),
            live_runner_id: present(&record.live_runner_id),
            terminal_session_id: present(&record.terminal_session_id),
            phase: ChatPhase::from_wire(&record.phase, &record.live_runner_id),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use daemonapi::frame::decode;

    use crate::test_support::state;

    #[test]
    fn a_chat_reads_its_surface_runner_session_and_phase() {
        let (mut state, _) = state();
        let data = r#"{"id":"hc","surface":"terminal","liveRunnerId":"r1","terminalSessionId":"t1","phase":"switching"}"#;
        let frame = format!(
            r#"{{"t":"entity","scope":"projects","kind":"chat","id":"hc","v":9,"data":{data}}}"#
        );
        state.apply("local", decode(&frame).unwrap());
        assert_eq!(
            ChatTerminal::of(&state, "hc"),
            Some(ChatTerminal {
                surface: ChatSurface::Terminal,
                live_runner_id: Some("r1".into()),
                terminal_session_id: Some("t1".into()),
                phase: ChatPhase::Switching,
            })
        );
        assert_eq!(ChatTerminal::of(&state, "gone"), None);
    }
}
