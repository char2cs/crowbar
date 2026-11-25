//! A native menu a pane page asked the shell for, and the page's wait for the answer.

use protocol::PaneId;
use webview::{ChosenEntry, MenuEntry};

/// One page's ask: the entries, the page point the menu opens at, and the answer the page
/// waits on. Dropped unanswered, it answers that nothing was chosen.
pub struct MenuAsk {
    /// The pane whose page asked.
    pub pane: PaneId,
    /// The menu, top to bottom.
    pub entries: Vec<MenuEntry>,
    /// Where it opens, in the page's own pixels from its top-left corner.
    pub at: (f64, f64),
    answer: Option<ChosenEntry>,
}

impl std::fmt::Debug for MenuAsk {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("MenuAsk")
            .field("pane", &self.pane)
            .field("entries", &self.entries)
            .field("at", &self.at)
            .finish_non_exhaustive()
    }
}

impl MenuAsk {
    /// The ask of `pane`'s page for `entries` at a page point; `answer` hears the choice.
    #[must_use]
    pub fn new(
        pane: PaneId,
        (entries, at): (Vec<MenuEntry>, (f64, f64)),
        answer: ChosenEntry,
    ) -> Self {
        Self {
            pane,
            entries,
            at,
            answer: Some(answer),
        }
    }

    /// Tells the page which entry was chosen: its index, then its index in each submenu it
    /// lies in; `None` when the menu was dismissed.
    pub fn answer(
        mut self,
        chosen: Option<Vec<usize>>,
    ) {
        if let Some(answer) = self.answer.take() {
            answer(chosen);
        }
    }
}

impl Drop for MenuAsk {
    fn drop(&mut self) {
        if let Some(answer) = self.answer.take() {
            answer(None);
        }
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use super::*;

    type Heard = Arc<Mutex<Vec<Option<Vec<usize>>>>>;

    fn asked() -> (MenuAsk, Heard) {
        let heard: Heard = Arc::default();
        let sink = Arc::clone(&heard);
        let answer: ChosenEntry = Box::new(move |path| sink.lock().unwrap().push(path));
        let pane = PaneId::new("p1").unwrap();
        (MenuAsk::new(pane, (Vec::new(), (0.0, 0.0)), answer), heard)
    }

    #[test]
    fn the_page_hears_one_answer_whether_the_shell_answers_or_drops_the_ask() {
        let (answered, heard_answer) = asked();
        answered.answer(Some(vec![1, 0]));
        let (dropped, heard_drop) = asked();
        drop(dropped);
        assert_eq!(*heard_answer.lock().unwrap(), [Some(vec![1, 0])]);
        assert_eq!(*heard_drop.lock().unwrap(), [None]);
    }
}
