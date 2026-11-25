use std::rc::Rc;

use crate::terminal_owner::TerminalOwner;

pub(crate) type HostOf = Rc<dyn Fn(&str) -> Option<String>>;

#[derive(Clone)]
pub(crate) struct ChatHosts {
    chat: HostOf,
    project: HostOf,
}

impl ChatHosts {
    pub(crate) fn new((chat, project): (HostOf, HostOf)) -> Self {
        Self { chat, project }
    }

    pub(crate) fn of(data: &Rc<store::Store>) -> Self {
        let (chats, projects) = (Rc::clone(data), Rc::clone(data));
        Self::new((
            Rc::new(move |chat| chats.chat_host(chat)),
            Rc::new(move |project| projects.project_host(project)),
        ))
    }

    pub(crate) fn host(
        &self,
        owner: &TerminalOwner,
    ) -> Option<String> {
        match owner {
            TerminalOwner::Chat(chat) => (self.chat)(chat),
            TerminalOwner::Home(project) => (self.project)(project),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn on_h2(id: &str) -> Option<String> {
        (id == "remote").then(|| "h2".to_owned())
    }

    #[test]
    fn a_terminal_runs_on_the_host_its_chat_or_its_project_lives_on() {
        let hosts = ChatHosts::new((Rc::new(on_h2), Rc::new(on_h2)));
        let chat = TerminalOwner::Chat("remote".into());
        let home = TerminalOwner::Home("remote".into());
        assert_eq!(
            hosts.host(&chat).as_deref(),
            Some("h2"),
            "a paired host's chat"
        );
        assert_eq!(
            hosts.host(&home).as_deref(),
            Some("h2"),
            "a paired host's project"
        );
        assert_eq!(
            hosts.host(&TerminalOwner::Home("local".into())),
            None,
            "this machine"
        );
    }
}
