use std::rc::Rc;

use store::ChatTerminal;

pub(crate) type Lookup = Rc<dyn Fn(&str) -> Option<ChatTerminal>>;
pub(crate) type Resume = Rc<dyn Fn(&str)>;
pub(crate) type HostOf = Rc<dyn Fn(&str) -> Option<String>>;

#[derive(Clone)]
pub(crate) struct ChatTerminals {
    lookup: Lookup,
    resume: Resume,
    host: HostOf,
}

impl ChatTerminals {
    pub(crate) fn new(
        lookup: Lookup,
        (resume, host): (Resume, HostOf),
    ) -> Self {
        Self {
            lookup,
            resume,
            host,
        }
    }

    pub(crate) fn of(data: &Rc<store::Store>) -> Self {
        let held = Rc::clone(data);
        let host: HostOf = Rc::new(move |chat| held.chat_host(chat));
        Self::new(reader(data), (starter(data), host))
    }

    pub(crate) fn host(
        &self,
        chat: &str,
    ) -> Option<String> {
        (self.host)(chat)
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

fn reader(data: &Rc<store::Store>) -> Lookup {
    let data = Rc::clone(data);
    Rc::new(move |chat| data.chat_terminal(chat))
}

fn starter(data: &Rc<store::Store>) -> Resume {
    let data = Rc::clone(data);
    Rc::new(move |chat| resume_chat(&data, chat))
}

fn resume_chat(
    data: &store::Store,
    chat: &str,
) {
    if let Err(error) = data.commands().resume_chat(chat) {
        tracing::warn!(%error, "the chat could not be resumed");
    }
}

#[cfg(test)]
mod tests {
    use store::{ChatPhase, ChatSurface};

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
        ChatTerminals::new(
            Rc::new(move |_| state.clone()),
            (Rc::new(|_| {}), Rc::new(|_| None)),
        )
    }

    #[test]
    fn a_fixed_state_is_read_back_for_any_chat() {
        for session in CASES {
            let state = session.map(live);
            assert_eq!(fixed(state.clone()).state("any"), state);
        }
    }
}
