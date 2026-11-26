use std::path::PathBuf;
use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AsyncApp, ClipboardItem, Context, EventEmitter, FontWeight, InteractiveElement as _,
    IntoElement, ParentElement, Render, StatefulInteractiveElement as _, Styled, Task, WeakEntity,
    Window, div, px,
};
use ui::{Button, ButtonSize, ButtonVariant, Surface, Theme, ToastContent, ToastKind};

use crate::daemon::daemon_link::DaemonLink;

const TITLE: &str = "Crowbar's backend keeps crashing";
const EXPLANATION: &str = "Crowbar stopped restarting it on its own. Your open tabs and unsaved edits are kept; nothing reconnects until the backend runs again.";
const COPIED: &str = "Diagnostics copied";
const NOT_OPENED: &str = "Could not open the log";
const NO_LOG: &str = "the daemon log's location is unknown";
const INSET: f32 = 16.0;
const WIDTH: f32 = 448.0;
const GAP: f32 = 12.0;
const BUTTON_GAP: f32 = 8.0;
const RADIUS: f32 = 8.0;
const REPORT_PADDING: f32 = 8.0;
const REPORT_RADIUS: f32 = 4.0;
const REPORT_MAX_HEIGHT: f32 = 160.0;
const REPORT_TEXT: f32 = 11.0;
const TITLE_TEXT: f32 = 14.0;
const BODY_TEXT: f32 = 12.0;
const REPORT_LINE: f32 = 16.5;
const TITLE_LINE: f32 = 20.0;
const BODY_LINE: f32 = 16.0;
const BORDER_ALPHA: f32 = 0.3;
const WELL_ALPHA: f32 = 0.5;

pub(crate) struct CrashPanel {
    report: String,
    link: Rc<DaemonLink>,
    restarting: Option<Task<()>>,
    failure: Option<String>,
}

impl EventEmitter<ToastContent> for CrashPanel {}

impl CrashPanel {
    pub(crate) fn new(
        report: String,
        link: Rc<DaemonLink>,
    ) -> Self {
        Self {
            report,
            link,
            restarting: None,
            failure: None,
        }
    }

    pub(crate) fn copy_report(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        cx.write_to_clipboard(ClipboardItem::new_string(self.report.clone()));
        cx.emit(ToastContent::new(ToastKind::Success, COPIED));
    }

    fn open_log(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let opened = self
            .link
            .log()
            .ok_or_else(|| NO_LOG.to_owned())
            .and_then(|log: PathBuf| {
                platform::reveal_in_file_manager(&log).map_err(|e| e.to_string())
            });
        if let Err(reason) = opened {
            cx.emit(ToastContent::new(ToastKind::Error, reason).titled(NOT_OPENED));
        }
    }

    pub(crate) fn restart(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.failure = None;
        let answered = self.link.restart();
        self.restarting =
            Some(cx.spawn(async move |panel, cx| hear_restart(panel, answered, cx).await));
        cx.notify();
    }

    fn restarted(
        &mut self,
        answer: Result<(), supervisor::Error>,
        cx: &mut Context<'_, Self>,
    ) {
        self.restarting = None;
        self.failure = answer.err().map(|error| error.to_string());
        cx.notify();
    }

    fn buttons(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let restart = Button::new("crash-restart", "Restart backend")
            .size(ButtonSize::Sm)
            .when(self.restarting.is_some(), Button::loading)
            .on_click(cx.listener(|panel, _, _, cx| panel.restart(cx)));
        div()
            .flex()
            .flex_wrap()
            .justify_end()
            .gap(px(BUTTON_GAP))
            .child(
                ghost("crash-copy", "Copy diagnostics")
                    .on_click(cx.listener(|panel, _, _, cx| panel.copy_report(cx))),
            )
            .child(
                ghost("crash-log", "Open log")
                    .on_click(cx.listener(|panel, _, _, cx| panel.open_log(cx))),
            )
            .child(restart)
    }
}

async fn hear_restart(
    panel: WeakEntity<CrashPanel>,
    answered: async_channel::Receiver<Result<(), supervisor::Error>>,
    cx: &mut AsyncApp,
) {
    let answer = answered.recv().await.unwrap_or(Ok(()));
    if panel
        .update(cx, |panel, cx| panel.restarted(answer, cx))
        .is_err()
    {
        tracing::debug!("the crash panel closed before the restart answered");
    }
}

fn ghost(
    id: &'static str,
    label: &'static str,
) -> Button {
    Button::new(id, label)
        .variant(ButtonVariant::Ghost)
        .size(ButtonSize::Sm)
}

impl CrashPanel {
    fn report_well(
        &self,
        cx: &Context<'_, Self>,
    ) -> impl IntoElement {
        let theme = Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        div()
            .id("crash-report")
            .max_h(px(REPORT_MAX_HEIGHT))
            .overflow_y_scroll()
            .rounded(px(REPORT_RADIUS))
            .bg(palette.muted().opacity(WELL_ALPHA))
            .p(px(REPORT_PADDING))
            .font_family(theme.mono_family())
            .text_size(px(REPORT_TEXT))
            .line_height(px(REPORT_LINE))
            .text_color(palette.muted_foreground())
            .child(self.report.clone())
    }

    fn failure_line(
        &self,
        cx: &Context<'_, Self>,
    ) -> Option<impl IntoElement> {
        let reason = self.failure.as_ref()?;
        let palette = Theme::of(cx).palette(Surface::Window);
        Some(
            div()
                .text_size(px(BODY_TEXT))
                .line_height(px(BODY_LINE))
                .text_color(palette.destructive())
                .child(format!("Restart failed: {reason}")),
        )
    }
}

fn heading(cx: &gpui::App) -> impl IntoElement {
    let palette = Theme::of(cx).palette(Surface::Window);
    let title = div()
        .text_size(px(TITLE_TEXT))
        .line_height(px(TITLE_LINE))
        .font_weight(FontWeight::MEDIUM)
        .text_color(palette.destructive())
        .child(TITLE);
    let explanation = div()
        .text_size(px(BODY_TEXT))
        .line_height(px(BODY_LINE))
        .text_color(palette.muted_foreground())
        .child(EXPLANATION);
    div()
        .flex()
        .flex_col()
        .gap_1()
        .child(title)
        .child(explanation)
}

impl Render for CrashPanel {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        div()
            .id("daemon-crash-panel")
            .debug_selector(|| "daemon-crash-panel".into())
            .absolute()
            .bottom(px(INSET))
            .right(px(INSET))
            .w(px(WIDTH))
            .flex()
            .flex_col()
            .gap(px(GAP))
            .rounded(px(RADIUS))
            .border_1()
            .border_color(palette.destructive().opacity(BORDER_ALPHA))
            .bg(palette.popover())
            .text_color(palette.foreground())
            .shadow_lg()
            .p(px(INSET))
            .occlude()
            .child(heading(cx))
            .child(self.report_well(cx))
            .children(self.failure_line(cx))
            .child(self.buttons(cx))
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;
    use crate::test_support::link_without_daemon;

    #[gpui::test]
    fn copying_the_diagnostics_puts_the_report_on_the_clipboard_and_says_so(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| ui::init(cx).unwrap());
        let link = Rc::new(link_without_daemon());
        let panel = cx.new(|_| CrashPanel::new("The backend exited 4 times.".into(), link));
        let (heard, said) = async_channel::unbounded();
        let hear = move |_, toast: &ToastContent, _: &mut gpui::App| {
            heard.try_send(toast.clone()).unwrap();
        };
        cx.update(|cx| cx.subscribe(&panel, hear).detach());
        panel.update(cx, CrashPanel::copy_report);
        let copied = cx.read_from_clipboard().and_then(|item| item.text());
        assert_eq!(copied.as_deref(), Some("The backend exited 4 times."));
        assert_eq!(
            said.try_recv(),
            Ok(ToastContent::new(ToastKind::Success, COPIED))
        );
    }
}
