use std::time::Duration;

use gpui::{Context, TaskExt as _};
use sidebar::CopyNote;

use crate::sidebar::Sidebar;

const SHOWN_FOR: Duration = Duration::from_millis(1500);

impl Sidebar {
    pub(crate) fn flash_copied(
        &mut self,
        copied: (String, CopyNote),
        cx: &mut Context<'_, Self>,
    ) {
        self.copied = Some(copied.clone());
        cx.notify();
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(SHOWN_FOR).await;
            this.update(cx, |this, cx| this.end_flash(&copied, cx))
        })
        .detach_and_log_err(cx);
    }

    fn end_flash(
        &mut self,
        copied: &(String, CopyNote),
        cx: &mut Context<'_, Self>,
    ) {
        if self.copied.as_ref() != Some(copied) {
            return;
        }
        self.copied = None;
        cx.notify();
    }
}

#[cfg(test)]
mod tests {
    use sidebar::CopyNote;

    use crate::sidebar::RowAction;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn a_copied_path_is_noted_on_its_row_for_a_moment(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let copied = RowAction::Copied("README.md".into(), CopyNote::Path);
        sidebar.update_in(cx, |this, window, cx| this.act(copied, window, cx));
        let noted =
            |cx: &mut gpui::VisualTestContext| sidebar.read_with(cx, |this, _| this.copied.clone());
        assert_eq!(noted(cx), Some(("README.md".into(), CopyNote::Path)));
        cx.executor().advance_clock(super::SHOWN_FOR);
        cx.run_until_parked();
        assert_eq!(noted(cx), None);
    }
}
