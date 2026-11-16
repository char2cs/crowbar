use gpui::{AppContext as _, Context, Entity, Window};
use sidebar::{Sidebar, Visibility};
use startup_view::StartupInput;
use ui::{ToastContent, ToastKind, ToastLifetime, ToastQueue};

use super::ShellView;
use crate::daemon_notice::{DaemonNotice, daemon_notice};
use crate::notice_key::NoticeKey;
use crate::notices::floating_at;
use crate::save_report::SaveReport;

pub(super) fn build_notices(
    sidebar: &Entity<Sidebar>,
    cx: &mut Context<'_, ShellView>,
) -> Entity<ToastQueue> {
    let side = sidebar.read(cx).preferences().side;
    cx.new(|_| ToastQueue::new(floating_at(side)))
}

impl ShellView {
    pub(super) fn track_daemon(
        &mut self,
        input: &StartupInput,
        cx: &mut Context<'_, Self>,
    ) {
        match daemon_notice(input) {
            DaemonNotice::Clear => self.clear_notice(NoticeKey::Daemon, cx),
            DaemonNotice::Show(text) => {
                self.show_notice(NoticeKey::Daemon, (ToastKind::Error, text), cx);
            },
        }
    }

    pub(super) fn announce_start(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(text) = self.state.take_notice() else {
            return;
        };
        self.show_notice(NoticeKey::Layout, (ToastKind::Warning, text), cx);
    }

    pub(super) fn on_save_report(
        &mut self,
        report: SaveReport,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match report {
            SaveReport::Failed(reason) => {
                let text = format!("Your layout could not be saved: {reason}");
                self.show_notice(NoticeKey::Storage, (ToastKind::Error, text), cx);
            },
            SaveReport::Recovered => self.clear_notice(NoticeKey::Storage, cx),
        }
    }

    pub(super) fn show_notice(
        &mut self,
        key: NoticeKey,
        (kind, text): (ToastKind, String),
        cx: &mut Context<'_, Self>,
    ) {
        let content = ToastContent::new(kind, text);
        let lifetime = key.lifetime();
        let preferences = self.sidebar.read(cx).preferences();
        if shows_in_sidebar(preferences.visibility) {
            self.toast_in_sidebar(key, (content, lifetime), cx);
            return;
        }
        self.notices.update(cx, |queue, cx| {
            queue.show(key.slug(), (content, lifetime), cx);
        });
    }

    fn toast_in_sidebar(
        &self,
        key: NoticeKey,
        (content, lifetime): (ToastContent, ToastLifetime),
        cx: &mut Context<'_, Self>,
    ) {
        let slug = Some(key.slug());
        self.sidebar.update(cx, |sidebar, cx| {
            sidebar.toast(content, (slug, lifetime), cx);
        });
    }

    pub(super) fn clear_notice(
        &mut self,
        key: NoticeKey,
        cx: &mut Context<'_, Self>,
    ) {
        self.sidebar
            .update(cx, |sidebar, cx| sidebar.clear_toast(key.slug(), cx));
        self.notices
            .update(cx, |queue, cx| queue.clear(key.slug(), cx));
    }

    pub(super) fn place_notices(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let side = self.sidebar.read(cx).preferences().side;
        self.notices
            .update(cx, |queue, cx| queue.set_placement(floating_at(side), cx));
    }
}

const fn shows_in_sidebar(visibility: Visibility) -> bool {
    matches!(visibility, Visibility::Open)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn notices_go_to_the_sidebar_only_while_it_is_open() {
        const CASES: &[(Visibility, bool)] =
            &[(Visibility::Open, true), (Visibility::Closed, false)];
        for (visibility, inside) in CASES {
            assert_eq!(shows_in_sidebar(*visibility), *inside, "{visibility:?}");
        }
    }
}
