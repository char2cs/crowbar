use std::rc::Rc;

pub(crate) type HostOf = Rc<dyn Fn(&str) -> Option<String>>;

#[derive(Clone)]
pub(crate) struct ChatHosts {
    host: HostOf,
}

impl ChatHosts {
    pub(crate) fn new(host: HostOf) -> Self {
        Self { host }
    }

    pub(crate) fn of(data: &Rc<store::Store>) -> Self {
        let held = Rc::clone(data);
        Self::new(Rc::new(move |chat| held.chat_host(chat)))
    }

    pub(crate) fn host(
        &self,
        chat: &str,
    ) -> Option<String> {
        (self.host)(chat)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_chat_s_terminal_runs_on_the_host_the_chat_lives_on() {
        let hosts = ChatHosts::new(Rc::new(|chat| (chat == "remote").then(|| "h2".to_owned())));
        assert_eq!(hosts.host("remote").as_deref(), Some("h2"), "a paired host");
        assert_eq!(hosts.host("local"), None, "this machine");
    }
}
