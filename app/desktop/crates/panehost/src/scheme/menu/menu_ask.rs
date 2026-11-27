use protocol::PaneId;
use webview::{ChosenEntry, MenuEntry};

pub struct MenuAsk {
    pub pane: PaneId,
    pub entries: Vec<MenuEntry>,
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
