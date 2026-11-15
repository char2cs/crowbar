use std::rc::Rc;

use shell_data::{ChatTerminal, ShellData};

pub(crate) type Lookup = Rc<dyn Fn(&str) -> Option<ChatTerminal>>;
pub(crate) type Resume = Rc<dyn Fn(&str)>;

#[derive(Clone)]
pub(crate) struct ChatTerminals {
    lookup: Lookup,
    resume: Resume,
}

impl ChatTerminals {
    pub(crate) fn new(
        lookup: Lookup,
        resume: Resume,
    ) -> Self {
        Self { lookup, resume }
    }

    pub(crate) fn of(data: &Rc<ShellData>) -> Self {
        Self::new(reader(data), starter(data))
    }

    pub(crate) fn state(
        &self,
        chat: &str,
    ) -> Option<ChatTerminal> {
        (self.lookup)(chat)
    }

    pub(crate) fn resume(
        &self,
        chat: &str,
    ) {
        (self.resume)(chat);
    }
}

fn reader(data: &Rc<ShellData>) -> Lookup {
    let data = Rc::clone(data);
    Rc::new(move |chat| data.chat_terminal(chat))
}

fn starter(data: &Rc<ShellData>) -> Resume {
    let data = Rc::clone(data);
    Rc::new(move |chat| resume_chat(&data, chat))
}

fn resume_chat(
    data: &ShellData,
    chat: &str,
) {
    if let Err(error) = data.commands().resume_chat(chat) {
        tracing::warn!(%error, "the chat could not be resumed");
    }
}

#[cfg(test)]
mod tests {
    use shell_data::{ChatPhase, ChatSurface};

    use super::*;

    const CASES: [Option<&str>; 2] = [None, Some("s1")];

    fn live(id: &str) -> ChatTerminal {
        ChatTerminal {
            surface: ChatSurface::Terminal,
            live_runner_id: Some("r".into()),
            terminal_session_id: Some(id.into()),
            phase: ChatPhase::Live,
        }
    }

    fn fixed(state: Option<ChatTerminal>) -> ChatTerminals {
        ChatTerminals::new(Rc::new(move |_| state.clone()), Rc::new(|_| {}))
    }

    #[test]
    fn a_fixed_state_is_read_back_for_any_chat() {
        for session in CASES {
            let state = session.map(live);
            assert_eq!(fixed(state.clone()).state("any"), state);
        }
    }
}
