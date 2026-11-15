use gpui::{
    AnyElement, Context, Div, InteractiveElement, IntoElement, ParentElement, SharedString,
    Stateful, StatefulInteractiveElement, Styled, div, px,
};
use startup_view::StartupInput;
use ui::{Toast, ToastKind};

use super::ShellView;
use crate::daemon_notice::{DaemonNotice, daemon_notice};
use crate::notice_key::NoticeKey;
use crate::notices::Notices;

const NOTICE_MARGIN: f32 = 16.0;
const NOTICE_GAP: f32 = 8.0;

impl ShellView {
    pub(super) fn track_daemon(
        &mut self,
        input: &StartupInput,
        cx: &mut Context<'_, Self>,
    ) {
        note_daemon(&mut self.notices, input);
        cx.notify();
    }

    pub(super) fn announce_start(&mut self) {
        let Some(text) = self.state.take_notice() else {
            return;
        };
        self.notices
            .set(NoticeKey::Layout, ToastKind::Warning, text);
    }

    pub(super) fn show_notice(
        &mut self,
        key: NoticeKey,
        (kind, text): (ToastKind, String),
        cx: &mut Context<'_, Self>,
    ) {
        self.notices.set(key, kind, text);
        cx.notify();
    }

    pub(super) fn notices_element(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        if self.notices.is_empty() {
            return None;
        }
        let toasts = self
            .notices
            .iter()
            .map(|(key, kind, text)| notice_card((*key, *kind, text.clone()), cx));
        Some(
            div()
                .absolute()
                .bottom(px(NOTICE_MARGIN))
                .right(px(NOTICE_MARGIN))
                .flex()
                .flex_col()
                .gap(px(NOTICE_GAP))
                .children(toasts)
                .into_any_element(),
        )
    }
}

fn note_daemon(
    notices: &mut Notices,
    input: &StartupInput,
) {
    match daemon_notice(input) {
        DaemonNotice::Clear => notices.clear(NoticeKey::Daemon),
        DaemonNotice::Show(text) => notices.set(NoticeKey::Daemon, ToastKind::Error, text),
    }
}

fn notice_card(
    (key, kind, text): (NoticeKey, ToastKind, SharedString),
    cx: &mut Context<'_, ShellView>,
) -> Stateful<Div> {
    div()
        .id(("notice", key as usize))
        .on_click(cx.listener(move |this, _, _, cx| {
            this.notices.clear(key);
            cx.notify();
        }))
        .child(Toast::new(("toast", key as usize), kind, text))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_daemon_outage_raises_a_notice_and_recovery_clears_it() {
        let mut notices = Notices::default();
        let halted = StartupInput::Halted {
            reason: None,
            message: "boom".into(),
        };
        note_daemon(&mut notices, &halted);
        note_daemon(&mut notices, &halted);
        assert_eq!(notices.iter().count(), 1, "one notice, not stacked");
        note_daemon(&mut notices, &StartupInput::Ready);
        assert!(notices.is_empty(), "recovered");
    }
}
