use gpui::Context;
use layout::Tab;

use crate::shell::shell_view::ShellView;

impl ShellView {
    pub(crate) fn show_active_space(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let shown = self.state.showing().map(Tab::id);
        if shown == self.layout.read(cx).shown_tab() {
            self.push_tabs(cx);
            return;
        }
        self.refresh(cx);
    }
}

#[cfg(test)]
mod tests {
    use layout::TabId;

    use super::*;
    use crate::shell::shell_view::tests::{Opened, SPACE, open_shell};

    fn shown(opened: &mut Opened) -> (Option<TabId>, bool) {
        opened.shell.read_with(&opened.window, |shell, cx| {
            let tab = shell.layout.read(cx).shown_tab();
            (tab, shell.tabs.read(cx).has_tabs())
        })
    }

    fn switch_to(
        opened: &mut Opened,
        space: &str,
    ) {
        opened.space.select(space);
        let shell = opened.shell.clone();
        shell.update(&mut opened.window, ShellView::on_data_changed);
        opened.window.run_until_parked();
    }

    #[gpui::test]
    fn switching_space_shows_that_space_s_tab_or_its_empty_stage(cx: &mut gpui::TestAppContext) {
        let mut opened = open_shell(cx);
        let (first, listed) = shown(&mut opened);
        assert!(first.is_some() && listed, "p1 shows its chat");
        switch_to(&mut opened, "p2");
        assert_eq!(
            shown(&mut opened),
            (None, false),
            "p2: the stage, no strip tab"
        );
        switch_to(&mut opened, SPACE);
        assert_eq!(shown(&mut opened), (first, true), "p1 again");
    }
}
