use std::cell::{Cell, RefCell};
use std::collections::BTreeMap;

use store::{Tool, ToolReport};

use crate::check_source::CheckSource;

#[derive(Default)]
pub(crate) struct FakeCheck {
    reports: RefCell<BTreeMap<Tool, ToolReport>>,
    reruns: Cell<u32>,
}

impl FakeCheck {
    pub(crate) fn answer(
        &self,
        tool: Tool,
        report: ToolReport,
    ) {
        self.reports.borrow_mut().insert(tool, report);
    }

    pub(crate) fn reruns(&self) -> u32 {
        self.reruns.get()
    }
}

impl CheckSource for FakeCheck {
    fn report(
        &self,
        tool: Tool,
    ) -> Option<ToolReport> {
        self.reports.borrow().get(&tool).cloned()
    }

    fn rerun(&self) {
        self.reruns.set(self.reruns.get() + 1);
    }
}
