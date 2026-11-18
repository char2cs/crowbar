//! Showing toasts at the bottom of the sidebar column.

use gpui::{AnyElement, Context, IntoElement};
use ui::{ToastContent, ToastLifetime};

use crate::sidebar::Sidebar;

impl Sidebar {
    /// Shows a toast over the bottom of the sidebar; a toast with the same `key` is rewritten in
    /// place, and an automatic one goes away after a few seconds.
    pub fn toast(
        &mut self,
        content: ToastContent,
        (key, lifetime): (Option<&str>, ToastLifetime),
        cx: &mut Context<'_, Self>,
    ) {
        self.toasts.update(cx, |queue, cx| match key {
            Some(key) => {
                queue.show(key.to_owned(), (content, lifetime), cx);
            },
            None => {
                queue.push(content, lifetime, cx);
            },
        });
    }

    /// Takes down the toast shown under `key`, for example once the problem it reports is gone.
    pub fn clear_toast(
        &mut self,
        key: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.toasts.update(cx, |queue, cx| queue.clear(key, cx));
    }

    pub(crate) fn toast_layer(&self) -> AnyElement {
        self.toasts.clone().into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use ui::ToastKind;

    use super::*;
    use sidebar::DockSide;

    use crate::test_support::prelude::*;

    const FADE: Duration = Duration::from_millis(200);

    fn shown(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> usize {
        let queue = sidebar.read_with(cx, |this, _| this.toasts.clone());
        queue.read_with(cx, |queue, _| queue.len())
    }

    fn info(text: &str) -> ToastContent {
        ToastContent::new(ToastKind::Info, text.to_owned())
    }

    #[gpui::test]
    fn an_automatic_toast_goes_away_a_pinned_one_stays_and_clear_takes_it_down(
        cx: &mut TestAppContext
    ) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| {
            this.toast(info("Saved"), (None, ToastLifetime::Auto), cx);
            this.toast(
                info("Backend unavailable"),
                (Some("connection"), ToastLifetime::Pinned),
                cx,
            );
            this.toast(
                info("Still unavailable"),
                (Some("connection"), ToastLifetime::Pinned),
                cx,
            );
        });
        assert_eq!(shown(&sidebar, cx), 2, "the keyed toast was rewritten");
        cx.executor()
            .advance_clock(ui::timing::TOAST_TIMEOUT + FADE);
        cx.run_until_parked();
        assert_eq!(shown(&sidebar, cx), 1);
        sidebar.update(cx, |this, cx| this.clear_toast("connection", cx));
        cx.executor().advance_clock(FADE);
        cx.run_until_parked();
        assert_eq!(shown(&sidebar, cx), 0);
    }

    #[gpui::test]
    fn a_refused_command_shows_its_reason(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        let rename = Command::Rename {
            row: RowId::from("chat"),
            name: "main".into(),
        };
        sidebar.update(cx, |this, cx| this.send(rename, cx));
        assert_eq!(shown(&sidebar, cx), 1);
    }

    #[gpui::test]
    fn docking_to_the_other_edge_moves_the_column(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.dock_to(DockSide::Right, cx));
        let queue = sidebar.read_with(cx, |this, _| this.toasts.clone());
        let placement = queue.read_with(cx, |queue, _| queue.placement());
        assert_eq!(
            placement,
            crate::sidebarview::docked_toasts(DockSide::Right)
        );
    }
}
