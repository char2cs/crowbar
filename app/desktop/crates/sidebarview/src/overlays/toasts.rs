use gpui::{AnyElement, App, Context, Entity, IntoElement, ParentElement, Styled, div};
use sidebar::{DockSide, Panel, Visibility};
use ui::{Portal, ToastContent, ToastLifetime, ToastPlacement, ToastQueue, ToastSide};

use crate::sidebar::Sidebar;

impl Sidebar {
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

    pub fn clear_toast(
        &mut self,
        key: &str,
        cx: &mut Context<'_, Self>,
    ) {
        self.toasts.update(cx, |queue, cx| queue.clear(key, cx));
    }

    pub fn toast_queue(&self) -> Entity<ToastQueue> {
        self.toasts.clone()
    }

    pub(crate) fn toast_layer(
        &self,
        cx: &App,
    ) -> Option<AnyElement> {
        let queue = self.toasts.read(cx);
        if queue.is_empty() || !matches!(queue.placement(), ToastPlacement::Docked(_)) {
            return None;
        }
        let toasts = self.toasts.clone();
        let layer = Portal::here("sidebar-toasts", move |_, _| toasts.clone());
        Some(div().absolute().inset_0().child(layer).into_any_element())
    }

    pub fn toast_side(side: DockSide) -> ToastSide {
        match side {
            DockSide::Left => ToastSide::Left,
            DockSide::Right => ToastSide::Right,
        }
    }

    pub(crate) fn toast_placement(panel: Panel) -> ToastPlacement {
        let side = Self::toast_side(panel.side());
        if panel.visibility() == Visibility::Open {
            return ToastPlacement::Docked(side);
        }
        ToastPlacement::Floating(side)
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use ui::ToastKind;

    use super::*;

    use crate::test_support::prelude::*;

    const FADE: Duration = Duration::from_millis(200);

    fn shown(
        sidebar: &Entity<Sidebar>,
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
        assert_eq!(placement, ToastPlacement::Docked(ToastSide::Right));
    }

    #[gpui::test]
    fn hiding_the_sidebar_floats_its_toasts_at_the_window_corner_and_keeps_them(
        cx: &mut TestAppContext
    ) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update_in(cx, |this, window, cx| {
            this.toast(info("Saved"), (None, ToastLifetime::Pinned), cx);
            this.toggle(window, cx);
        });
        let queue = sidebar.read_with(cx, |this, _| this.toast_queue());
        let placement = queue.read_with(cx, |queue, _| queue.placement());
        assert_eq!(placement, ToastPlacement::Floating(ToastSide::Left));
        assert_eq!(shown(&sidebar, cx), 1, "kept");
        let drawn = sidebar.read_with(cx, |this, cx| this.toast_layer(cx).is_some());
        assert!(!drawn, "the shell draws a floating column");
        sidebar.update_in(cx, Sidebar::toggle);
        let placement = queue.read_with(cx, |queue, _| queue.placement());
        assert_eq!(placement, ToastPlacement::Docked(ToastSide::Left));
    }

    #[test]
    fn each_edge_swipes_toward_itself() {
        const CASES: [(DockSide, ToastSide); 2] = [
            (DockSide::Left, ToastSide::Left),
            (DockSide::Right, ToastSide::Right),
        ];
        for (side, toast) in CASES {
            assert_eq!(Sidebar::toast_side(side), toast, "{side:?}");
        }
    }

    #[gpui::test]
    fn a_toast_floats_above_the_panes_over_the_foot_of_the_column(cx: &mut TestAppContext) {
        let mut floating = crate::test_support::open_floating(cx, DemoShell::reference());
        let sidebar = floating.sidebar.clone();
        sidebar.update(&mut floating.window, |this, cx| {
            this.toast(info("Saved"), (None, ToastLifetime::Pinned), cx);
        });
        floating.window.run_until_parked();
        assert_eq!(
            floating.window.debug_bounds("ui-toast"),
            None,
            "nothing in the window"
        );
        let toast = floating.top().debug_bounds("ui-toast").unwrap_or_default();
        assert!(toast.right() <= px(294.0), "{toast:?} inside the column");
    }
}
