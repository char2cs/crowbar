use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    Context, Div, EventEmitter, FontWeight, IntoElement, ParentElement, Render, Styled, Window, div,
};
use shell_data::{SystemCheck, Tool};
use ui::{Button, ButtonSize, ButtonVariant, Icon, IconName, Theme, Wordmark};

use crate::backdrop::backdrop;
use crate::check_source::CheckSource;
use crate::check_status::{CheckStatus, description, label};
use crate::look::{
    BLOCK_GAP, BODY, CAPTION, CARD_LOGO, CARD_PADDING, CARD_RADIUS, CARD_WIDTH, CHECK_CAPTION,
    CHECK_TITLE, CHOOSE_LABEL, CONTINUE_LABEL, FAINT, GIT_BLOCKED, GLASS, HEADING, HOST_LABEL,
    HOST_LOGO, RETRY_LABEL, RIM, ROW_GAP, ROW_PAD, RULE, SOFT, SPACE_ABOUT, SPACE_MEMORY,
    SPACE_TITLE, STAGE_GAP, STATUS_ICON, TAGLINE, TAGLINE_INK, TAGLINE_SIZE, TAGLINE_WIDTH,
    WELCOME_LABEL, WELCOME_LOGO, black, white,
};
use crate::oobe_event::OobeEvent;
use crate::oobe_screen::OobeScreen;
use crate::pill_button::PillButton;

const DANGER: u32 = 0x00f8_7171;
const WARNING: u32 = 0x00fb_bf24;
const SUCCESS: u32 = 0x0034_d399;

/// The first-run screens over the brand gradient.
pub struct OobeView {
    screen: OobeScreen,
    source: Option<Rc<dyn CheckSource>>,
}

impl std::fmt::Debug for OobeView {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("OobeView")
            .field("screen", &self.screen)
            .finish_non_exhaustive()
    }
}

impl EventEmitter<OobeEvent> for OobeView {}

impl OobeView {
    /// The first run of a computer with a daemon of its own, starting on the brand.
    #[must_use]
    pub fn first_run(check: Arc<SystemCheck>) -> Self {
        Self::over(OobeScreen::Welcome, Some(Rc::new(check)))
    }

    /// The way in for an install with no daemon of its own: connect to a host.
    #[must_use]
    pub fn add_host() -> Self {
        Self::over(OobeScreen::AddHost, None)
    }

    fn over(
        screen: OobeScreen,
        source: Option<Rc<dyn CheckSource>>,
    ) -> Self {
        Self { screen, source }
    }

    /// The screen showing now.
    #[must_use]
    pub fn screen(&self) -> OobeScreen {
        self.screen
    }

    /// Tells the view the system check answered, so it redraws.
    pub fn changed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        cx.notify();
    }

    fn git_ready(&self) -> bool {
        self.status_of(Tool::Git) == CheckStatus::Ready
    }

    fn status_of(
        &self,
        tool: Tool,
    ) -> CheckStatus {
        let report = self.source.as_ref().and_then(|source| source.report(tool));
        CheckStatus::of(tool, report.as_ref())
    }

    fn advance(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.screen == OobeScreen::SystemCheck && !self.git_ready() {
            return;
        }
        self.screen = self.screen.next();
        cx.notify();
    }

    fn request_host(cx: &mut Context<'_, Self>) {
        cx.emit(OobeEvent::AddHost);
    }

    fn request_folder(cx: &mut Context<'_, Self>) {
        cx.emit(OobeEvent::ChooseFolder);
    }

    fn recheck(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if let Some(source) = &self.source {
            source.rerun();
        }
        cx.notify();
    }

    fn is_blocked(&self) -> bool {
        self.all_answered() && !self.git_ready()
    }

    fn all_answered(&self) -> bool {
        Tool::ALL
            .iter()
            .all(|tool| self.status_of(*tool) != CheckStatus::Checking)
    }

    fn welcome(cx: &mut Context<'_, Self>) -> Div {
        column(STAGE_GAP)
            .child(Wordmark::new(WELCOME_LOGO).color(white(1.0)))
            .child(
                div()
                    .max_w(TAGLINE_WIDTH)
                    .text_size(TAGLINE_SIZE)
                    .text_color(black(TAGLINE_INK))
                    .child(TAGLINE),
            )
            .child(PillButton::new(
                "oobe-start",
                WELCOME_LABEL,
                cx.listener(|this, _, _, cx| this.advance(cx)),
            ))
    }

    fn host(cx: &mut Context<'_, Self>) -> Div {
        column(STAGE_GAP)
            .child(Wordmark::new(HOST_LOGO).color(white(1.0)))
            .child(
                div()
                    .max_w(TAGLINE_WIDTH)
                    .text_size(TAGLINE_SIZE)
                    .text_color(white(TAGLINE_INK))
                    .child(TAGLINE),
            )
            .child(PillButton::new(
                "oobe-add-host",
                HOST_LABEL,
                cx.listener(|_, _, _, cx| Self::request_host(cx)),
            ))
    }

    fn card(body: Div) -> Div {
        div()
            .w(CARD_WIDTH)
            .p(CARD_PADDING)
            .rounded(CARD_RADIUS)
            .border_1()
            .border_color(white(RIM))
            .bg(white(GLASS))
            .flex()
            .flex_col()
            .items_center()
            .gap(STAGE_GAP)
            .child(Wordmark::new(CARD_LOGO).color(white(1.0)))
            .child(body)
    }

    fn system_check(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let rows: Vec<_> = Tool::ALL.iter().map(|tool| self.check_row(*tool)).collect();
        let note = self.is_blocked().then(|| {
            div()
                .text_size(CAPTION)
                .text_color(crate::look::color(DANGER))
                .child(GIT_BLOCKED)
        });
        let body = div()
            .w_full()
            .flex()
            .flex_col()
            .gap(BLOCK_GAP)
            .child(heading(CHECK_TITLE, cx))
            .child(
                div()
                    .text_size(CAPTION)
                    .text_color(white(FAINT))
                    .child(CHECK_CAPTION),
            )
            .children(rows)
            .children(note)
            .child(self.check_actions(cx));
        Self::card(body)
    }

    fn check_actions(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        let recheck = self.is_blocked().then(|| {
            Button::new("oobe-recheck", RETRY_LABEL)
                .icon(IconName::ArrowClockwise)
                .size(ButtonSize::Sm)
                .variant(ButtonVariant::Ghost)
                .on_click(cx.listener(|this, _, _, cx| this.recheck(cx)))
        });
        let mut proceed = PillButton::new(
            "oobe-continue",
            CONTINUE_LABEL,
            cx.listener(|this, _, _, cx| this.advance(cx)),
        );
        if !self.git_ready() {
            proceed = proceed.disabled();
        }
        div()
            .flex()
            .flex_row()
            .gap(BLOCK_GAP)
            .w_full()
            .children(recheck)
            .child(div().ml_auto().child(proceed))
    }

    fn check_row(
        &self,
        tool: Tool,
    ) -> Div {
        let status = self.status_of(tool);
        let report = self.source.as_ref().and_then(|source| source.report(tool));
        div()
            .flex()
            .flex_row()
            .items_center()
            .justify_between()
            .gap(ROW_GAP)
            .py(ROW_PAD)
            .border_b_1()
            .border_color(white(RULE))
            .child(
                div()
                    .flex()
                    .flex_col()
                    .child(
                        div()
                            .text_size(BODY)
                            .font_weight(FontWeight::MEDIUM)
                            .text_color(white(1.0))
                            .child(label(tool)),
                    )
                    .child(
                        div()
                            .text_size(CAPTION)
                            .text_color(white(FAINT))
                            .child(description(tool, report.as_ref())),
                    ),
            )
            .child(status_icon(status))
    }

    fn add_space(cx: &mut Context<'_, Self>) -> Div {
        let paragraph =
            |text: &'static str| div().text_size(BODY).text_color(white(SOFT)).child(text);
        let body = div()
            .w_full()
            .flex()
            .flex_col()
            .gap(STAGE_GAP)
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(BLOCK_GAP)
                    .child(heading(SPACE_TITLE, cx))
                    .child(paragraph(SPACE_ABOUT))
                    .child(paragraph(SPACE_MEMORY)),
            )
            .child(PillButton::new(
                "oobe-choose-folder",
                CHOOSE_LABEL,
                cx.listener(|_, _, _, cx| Self::request_folder(cx)),
            ));
        Self::card(body)
    }
}

fn column(gap: gpui::Pixels) -> Div {
    div()
        .flex()
        .flex_col()
        .items_center()
        .text_center()
        .gap(gap)
}

fn heading(
    text: &'static str,
    cx: &mut Context<'_, OobeView>,
) -> Div {
    div()
        .font_family(Theme::of(cx).heading_family())
        .text_size(HEADING)
        .text_color(white(1.0))
        .child(text)
}

fn status_icon(status: CheckStatus) -> Icon {
    let hue = match status {
        CheckStatus::Checking => white(FAINT),
        CheckStatus::Ready => crate::look::color(SUCCESS),
        CheckStatus::Optional => crate::look::color(WARNING),
        CheckStatus::Missing => crate::look::color(DANGER),
    };
    Icon::new(status.icon()).size(STATUS_ICON).color(hue)
}

impl Render for OobeView {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let content = match self.screen {
            OobeScreen::Welcome => Self::welcome(cx),
            OobeScreen::SystemCheck => self.system_check(cx),
            OobeScreen::AddSpace => Self::add_space(cx),
            OobeScreen::AddHost => Self::host(cx),
        };
        div()
            .size_full()
            .relative()
            .font_family(Theme::of(cx).ui_family())
            .child(backdrop(self.screen))
            .child(stage(content))
    }
}

fn stage(content: Div) -> Div {
    div()
        .absolute()
        .inset_0()
        .flex()
        .items_center()
        .justify_center()
        .child(content)
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;

    use gpui::{Entity, TestAppContext, VisualTestContext};

    use shell_data::ToolReport;

    use super::*;
    use crate::test_support::FakeCheck;

    fn open<'a>(
        cx: &'a mut TestAppContext,
        screen: OobeScreen,
        check: &Rc<FakeCheck>,
    ) -> (Entity<OobeView>, &'a mut VisualTestContext) {
        cx.update(|cx| ui::init(cx).expect("the design system installs"));
        let source: Rc<dyn CheckSource> = Rc::clone(check) as Rc<dyn CheckSource>;
        let (view, cx) = cx.add_window_view(move |_, _| OobeView::over(screen, Some(source)));
        cx.run_until_parked();
        (view, cx)
    }

    fn screen_of(
        view: &Entity<OobeView>,
        cx: &mut VisualTestContext,
    ) -> OobeScreen {
        view.read_with(cx, |view, _| view.screen())
    }

    fn collect(
        sink: Rc<RefCell<Vec<OobeEvent>>>
    ) -> impl FnMut(Entity<OobeView>, &OobeEvent, &mut gpui::App) + 'static {
        move |_, event, _| sink.borrow_mut().push(*event)
    }

    fn events(
        view: &Entity<OobeView>,
        cx: &mut VisualTestContext,
    ) -> Rc<RefCell<Vec<OobeEvent>>> {
        let seen = Rc::new(RefCell::new(Vec::new()));
        let sink = Rc::clone(&seen);
        cx.update(|_, cx| cx.subscribe(view, collect(sink)).detach());
        seen
    }

    #[test]
    fn the_stage_is_pinned_to_every_edge_because_a_percent_size_collapses_in_the_live_window() {
        let mut shown = stage(div());
        let inset = &shown.style().inset;
        let pinned = [inset.top, inset.left, inset.right, inset.bottom]
            .iter()
            .all(|edge| *edge == Some(gpui::px(0.0).into()));
        assert!(pinned, "{inset:?}");
    }

    #[gpui::test]
    fn the_welcome_button_leads_to_the_system_check(cx: &mut TestAppContext) {
        let check = Rc::new(FakeCheck::default());
        let (view, cx) = open(cx, OobeScreen::Welcome, &check);
        view.update(cx, OobeView::advance);
        assert_eq!(screen_of(&view, cx), OobeScreen::SystemCheck);
    }

    #[gpui::test]
    fn continue_waits_for_git_and_a_missing_git_offers_another_check(cx: &mut TestAppContext) {
        let check = Rc::new(FakeCheck::default());
        let (view, cx) = open(cx, OobeScreen::SystemCheck, &check);
        view.update(cx, OobeView::advance);
        assert_eq!(
            screen_of(&view, cx),
            OobeScreen::SystemCheck,
            "still probing"
        );
        for tool in Tool::ALL {
            check.answer(tool, ToolReport::default());
        }
        view.update(cx, OobeView::changed);
        cx.run_until_parked();
        view.update(cx, OobeView::advance);
        view.update(cx, OobeView::recheck);
        assert_eq!(
            screen_of(&view, cx),
            OobeScreen::SystemCheck,
            "git is missing"
        );
        assert!(check.reruns() > 0, "Check again asks the daemon again");
        check.answer(
            Tool::Git,
            ToolReport {
                installed: true,
                ..ToolReport::default()
            },
        );
        view.update(cx, OobeView::changed);
        cx.run_until_parked();
        view.update(cx, OobeView::advance);
        assert_eq!(screen_of(&view, cx), OobeScreen::AddSpace);
    }

    #[gpui::test]
    fn the_last_screens_ask_the_shell_for_what_they_offer(cx: &mut TestAppContext) {
        let check = Rc::new(FakeCheck::default());
        let (view, cx) = open(cx, OobeScreen::AddSpace, &check);
        let seen = events(&view, cx);
        view.update(cx, |_, cx| OobeView::request_folder(cx));
        view.update(cx, |_, cx| OobeView::request_host(cx));
        let expected = [OobeEvent::ChooseFolder, OobeEvent::AddHost];
        assert_eq!(seen.borrow().as_slice(), expected, "the two requests");
    }
}
