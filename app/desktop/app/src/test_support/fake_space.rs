use std::cell::RefCell;
use std::collections::BTreeMap;

use crate::active_space::ActiveSpace;

#[derive(Debug, Default)]
pub(crate) struct FakeSpace {
    active: RefCell<String>,
    chats: RefCell<BTreeMap<String, String>>,
}

impl FakeSpace {
    pub(crate) fn select(
        &self,
        space: &str,
    ) {
        space.clone_into(&mut self.active.borrow_mut());
    }

    pub(crate) fn know(
        &self,
        (chat, space): (&str, &str),
    ) {
        self.chats
            .borrow_mut()
            .insert(chat.to_owned(), space.to_owned());
    }
}

impl ActiveSpace for FakeSpace {
    fn active(&self) -> String {
        self.active.borrow().clone()
    }

    fn of_chat(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.chats.borrow().get(chat).cloned()
    }
}
