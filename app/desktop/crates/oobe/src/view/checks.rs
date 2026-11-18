//! What the system check says about the tools the flow needs.

use store::Tool;

use super::View;
use crate::check_status::CheckStatus;

impl View {
    pub(crate) fn git_ready(&self) -> bool {
        self.status_of(Tool::Git) == CheckStatus::Ready
    }

    pub(crate) fn status_of(
        &self,
        tool: Tool,
    ) -> CheckStatus {
        let report = self.source.as_ref().and_then(|source| source.report(tool));
        CheckStatus::of(tool, report.as_ref())
    }

    pub(crate) fn is_blocked(&self) -> bool {
        self.all_answered() && !self.git_ready()
    }

    pub(crate) fn all_answered(&self) -> bool {
        Tool::ALL
            .iter()
            .all(|tool| self.status_of(*tool) != CheckStatus::Checking)
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;
    use store::ToolReport;

    use super::*;
    use crate::screen::Screen;
    use crate::test_support::FakeCheck;
    use crate::view::tests::{open, screen_of};

    #[gpui::test]
    fn continue_waits_for_git_and_a_missing_git_offers_another_check(cx: &mut TestAppContext) {
        let check = Rc::new(FakeCheck::default());
        let (view, cx) = open(cx, Screen::SystemCheck, &check);
        view.update(cx, View::advance);
        assert_eq!(screen_of(&view, cx), Screen::SystemCheck, "still probing");
        for tool in Tool::ALL {
            check.answer(tool, ToolReport::default());
        }
        view.update(cx, View::changed);
        cx.run_until_parked();
        view.update(cx, View::advance);
        view.update(cx, View::recheck);
        assert_eq!(screen_of(&view, cx), Screen::SystemCheck, "git is missing");
        assert!(check.reruns() > 0, "Check again asks the daemon again");
        check.answer(
            Tool::Git,
            ToolReport {
                installed: true,
                ..ToolReport::default()
            },
        );
        view.update(cx, View::changed);
        cx.run_until_parked();
        view.update(cx, View::advance);
        assert_eq!(screen_of(&view, cx), Screen::AddSpace);
    }
}
