//! The queue of native menus pages ask the shell for.

use crate::scheme::menu::menu_ask::MenuAsk;

/// Carries each [`MenuAsk`] from the scheme, which hears the page, to the shell, which pops the
/// menu over the pane. Clones share one queue: the [`PaneProtocol`](crate::PaneProtocol) sends,
/// the [`PaneHost`](crate::PaneHost) hands the receiving end to the shell.
#[derive(Debug, Clone)]
pub struct PaneMenus {
    sender: async_channel::Sender<MenuAsk>,
    receiver: async_channel::Receiver<MenuAsk>,
}

impl Default for PaneMenus {
    fn default() -> Self {
        let (sender, receiver) = async_channel::unbounded();
        Self { sender, receiver }
    }
}

impl PaneMenus {
    /// The asks, in the order pages made them.
    #[must_use]
    pub fn asks(&self) -> async_channel::Receiver<MenuAsk> {
        self.receiver.clone()
    }

    pub(crate) fn send(
        &self,
        ask: MenuAsk,
    ) {
        if let Err(refused) = self.sender.try_send(ask) {
            refused.into_inner().answer(None);
        }
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use protocol::PaneId;

    use super::*;

    type Heard = Arc<Mutex<Vec<Option<Vec<usize>>>>>;

    fn ask(
        pane: &str,
        heard: &Heard,
    ) -> MenuAsk {
        let sink = Arc::clone(heard);
        let pane = PaneId::new(pane).unwrap();
        let answer = Box::new(move |path| sink.lock().unwrap().push(path));
        MenuAsk::new(pane, (Vec::new(), (0.0, 0.0)), answer)
    }

    #[test]
    fn asks_reach_the_shell_in_order_and_a_closed_queue_answers_nothing_chosen() {
        let heard: Heard = Arc::default();
        let menus = PaneMenus::default();
        menus.send(ask("p1", &heard));
        menus.send(ask("p2", &heard));
        let asks = menus.asks();
        let order: Vec<String> = std::iter::from_fn(|| asks.try_recv().ok())
            .map(|ask| ask.pane.as_str().to_owned())
            .collect();
        assert_eq!(order, ["p1", "p2"]);
        asks.close();
        menus.send(ask("p3", &heard));
        assert_eq!(heard.lock().unwrap().last(), Some(&None));
    }
}
