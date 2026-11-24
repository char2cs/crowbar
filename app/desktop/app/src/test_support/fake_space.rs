use std::cell::RefCell;

use crate::active_space::ActiveSpace;

#[derive(Debug, Default)]
pub(crate) struct FakeSpace(RefCell<String>);

impl FakeSpace {
    pub(crate) fn select(
        &self,
        space: &str,
    ) {
        space.clone_into(&mut self.0.borrow_mut());
    }
}

impl ActiveSpace for FakeSpace {
    fn active(&self) -> String {
        self.0.borrow().clone()
    }
}
