//! The action a native menu row dispatches when it is chosen; `answer` carries its path to the
//! menu's caller.

use async_channel::Sender;
use gpui::Action;

#[derive(Debug, Clone, Action)]
#[action(namespace = native_menu, no_json, no_register)]
pub(super) struct Chosen {
    pub(super) path: ui::MenuPath,
    pub(super) answer: Sender<ui::MenuPath>,
}

impl Chosen {
    pub(super) fn new(
        path: ui::MenuPath,
        answer: &Sender<ui::MenuPath>,
    ) -> Box<Self> {
        let answer = answer.clone();
        Box::new(Self { path, answer })
    }
}

impl PartialEq for Chosen {
    fn eq(
        &self,
        other: &Self,
    ) -> bool {
        self.path == other.path && self.answer.same_channel(&other.answer)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rows_are_the_same_only_on_one_path_of_one_menu() {
        let (one, _heard) = async_channel::bounded(1);
        let (other, _unheard) = async_channel::bounded(1);
        assert_eq!(Chosen::new(vec![1], &one), Chosen::new(vec![1], &one));
        assert_ne!(Chosen::new(vec![1], &one), Chosen::new(vec![2], &one));
        assert_ne!(Chosen::new(vec![1], &one), Chosen::new(vec![1], &other));
    }
}
