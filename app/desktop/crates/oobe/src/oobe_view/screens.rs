//! The four screens of the first-run flow.

use gpui::{Context, Div, FontWeight, ParentElement, Styled, div};
use store::Tool;
use ui::{Button, ButtonSize, ButtonVariant, IconName, Wordmark};

use super::OobeView;
use super::parts::{DANGER, column, heading, status_icon};
use crate::check_status::{description, label};
use crate::look::{
    BLOCK_GAP, BODY, CAPTION, CARD_LOGO, CARD_PADDING, CARD_RADIUS, CARD_WIDTH, CHECK_CAPTION,
    CHECK_TITLE, CHOOSE_LABEL, CONTINUE_LABEL, FAINT, GIT_BLOCKED, GLASS, HOST_LABEL, HOST_LOGO,
    RETRY_LABEL, RIM, ROW_GAP, ROW_PAD, RULE, SOFT, SPACE_ABOUT, SPACE_MEMORY, SPACE_TITLE,
    STAGE_GAP, TAGLINE, TAGLINE_INK, TAGLINE_SIZE, TAGLINE_WIDTH, WELCOME_LABEL, WELCOME_LOGO,
    black, white,
};
use crate::pill_button::PillButton;

impl OobeView {
    pub(crate) fn welcome(cx: &mut Context<'_, Self>) -> Div {
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

    pub(crate) fn host(cx: &mut Context<'_, Self>) -> Div {
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

    pub(crate) fn card(body: Div) -> Div {
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

    pub(crate) fn system_check(
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

    pub(crate) fn check_actions(
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

    pub(crate) fn check_row(
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

    pub(crate) fn add_space(cx: &mut Context<'_, Self>) -> Div {
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

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;

    use crate::oobe_screen::OobeScreen;
    use crate::oobe_view::tests::{open, screen_of};
    use crate::test_support::FakeCheck;

    #[gpui::test]
    fn every_screen_renders(cx: &mut TestAppContext) {
        let check = Rc::new(FakeCheck::default());
        for screen in [
            OobeScreen::Welcome,
            OobeScreen::SystemCheck,
            OobeScreen::AddSpace,
            OobeScreen::AddHost,
        ] {
            let (view, cx) = open(cx, screen, &check);
            assert_eq!(screen_of(&view, cx), screen);
        }
    }
}
