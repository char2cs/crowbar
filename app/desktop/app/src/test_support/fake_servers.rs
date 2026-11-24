//! Language servers that record what each chat's panes told them and answer `null`.

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{App, Task};
use serde_json::Value;

use crate::chat_servers::ChatServers;

type Told = Rc<RefCell<Vec<(String, &'static str, Value)>>>;

#[derive(Default, Clone)]
pub(crate) struct FakeServers {
    told: Told,
}

struct FakeServer {
    chat: String,
    told: Told,
}

impl FakeServers {
    pub(crate) fn servers(&self) -> ChatServers {
        let told = Rc::clone(&self.told);
        Rc::new(move |chat| FakeServer::of(chat, &told))
    }

    pub(crate) fn told(&self) -> Vec<(String, &'static str, Value)> {
        self.told.borrow().clone()
    }
}

impl FakeServer {
    fn of(
        chat: &str,
        told: &Told,
    ) -> Rc<dyn editorview::LanguageServer> {
        let (chat, told) = (chat.to_owned(), Rc::clone(told));
        Rc::new(Self { chat, told })
    }
}

impl editorview::LanguageServer for FakeServer {
    fn post(
        &self,
        verb: &'static str,
        body: Value,
        _: &mut App,
    ) -> Task<Result<Value, editorview::LspError>> {
        self.told.borrow_mut().push((self.chat.clone(), verb, body));
        Task::ready(Ok(Value::Null))
    }
}
