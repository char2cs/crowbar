//! Showing toasts at the bottom of the sidebar column.

use std::time::Duration;

use gpui::{AnyElement, Context, ElementId, IntoElement, ParentElement, Styled, div};
use ui::{Toast, ToastKind};

use crate::scale::SPACE_MD;
use crate::sidebar::Sidebar;

const TOAST_SPAN: Duration = Duration::from_secs(5);

impl Sidebar {
    /// Shows a toast over the bottom of the sidebar; a toast with the same `key` replaces the
    /// earlier one, and every toast goes away after a few seconds.
    pub fn toast(
        &mut self,
        (kind, message): (ToastKind, String),
        key: Option<&str>,
        cx: &mut Context<'_, Self>,
    ) {
        let id = self.toasts.push((kind, message), key);
        cx.spawn(async move |this, cx| {
            cx.background_executor().timer(TOAST_SPAN).await;
            this.update(cx, |this, cx| this.expire_toast(id, cx))
        })
        .detach_and_log_err(cx);
        cx.notify();
    }

    fn expire_toast(
        &mut self,
        id: u64,
        cx: &mut Context<'_, Self>,
    ) {
        self.toasts.expire(id);
        cx.notify();
    }

    pub(crate) fn toast_layer(&self) -> Option<AnyElement> {
        let shown = self.toasts.visible();
        if shown.is_empty() {
            return None;
        }
        let cards = shown.iter().map(|entry| {
            let id = (ElementId::from("sidebar-toast"), entry.id.to_string());
            Toast::new(id, entry.kind, entry.message.clone()).updated(entry.updates)
        });
        Some(
            div()
                .absolute()
                .bottom_0()
                .left_0()
                .right_0()
                .flex()
                .flex_col()
                .gap(SPACE_MD)
                .p(SPACE_MD)
                .children(cards)
                .into_any_element(),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    fn shown(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> usize {
        sidebar.read_with(cx, |this, _| this.toasts.visible().len())
    }

    #[gpui::test]
    fn a_toast_shows_then_goes_away(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, |this, cx| {
            this.toast((ToastKind::Info, "Saved".into()), None, cx);
        });
        assert_eq!(shown(&sidebar, cx), 1);
        cx.executor().advance_clock(TOAST_SPAN);
        cx.run_until_parked();
        assert_eq!(shown(&sidebar, cx), 0);
    }

    #[gpui::test]
    fn a_refused_command_shows_its_reason(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        let rename = Command::Rename {
            row: RowId::from("chat"),
            name: "main".into(),
        };
        sidebar.update(cx, |this, cx| this.send(rename, cx));
        let message = sidebar.read_with(cx, |this, _| {
            this.toasts
                .visible()
                .first()
                .map(|entry| entry.message.clone())
        });
        assert_eq!(message.as_deref(), Some("\"main\" already exists"));
    }
}
