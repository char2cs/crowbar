use std::sync::Arc;

use shell_data::{SystemCheck, Tool, ToolReport};

pub(crate) trait CheckSource {
    fn report(
        &self,
        tool: Tool,
    ) -> Option<ToolReport>;
    fn rerun(&self);
}

impl CheckSource for Arc<SystemCheck> {
    fn report(
        &self,
        tool: Tool,
    ) -> Option<ToolReport> {
        SystemCheck::report(self, tool)
    }

    fn rerun(&self) {
        SystemCheck::rerun(self);
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use super::*;
    use crate::test_support::FakeCheck;

    #[test]
    fn a_source_answers_per_tool_once_asked_and_can_be_asked_again() {
        let check = Rc::new(FakeCheck::default());
        let source: Rc<dyn CheckSource> = Rc::clone(&check) as Rc<dyn CheckSource>;
        assert_eq!(source.report(Tool::Git), None, "still probing");
        check.answer(
            Tool::Git,
            ToolReport {
                installed: true,
                ..ToolReport::default()
            },
        );
        assert!(
            source
                .report(Tool::Git)
                .is_some_and(|report| report.installed)
        );
        assert_eq!(source.report(Tool::Gh), None, "each tool answers alone");
        source.rerun();
        assert_eq!(check.reruns(), 1);
    }
}
