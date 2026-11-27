use std::rc::Rc;

use gpui::{
    App, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{
    Badge, BadgeVariant, Button, ButtonSize, Icon, IconName, IconWeight, ScaledText, Surface,
    Theme, radius,
};

use crate::actions::Actions;
use crate::command::Command;
use crate::report::HaltReason;

const WIDTH: f32 = 480.0;
const TINT: f32 = 0.1;
const TINT_BORDER: f32 = 0.2;
const GAP_SM: f32 = 8.0;
const GAP_MD: f32 = 12.0;
const CARD_PADDING: f32 = 20.0;
const TITLE_ICON: f32 = 20.0;
const DETAIL_MAX_HEIGHT: f32 = 160.0;
const DETAIL_TEXT_SIZE: f32 = 12.0;
const DETAIL_LINE_HEIGHT_RATIO: f32 = 1.5;
const DETAIL_PADDING_X: f32 = 10.0;
const DETAIL_PADDING_Y: f32 = 8.0;

#[derive(IntoElement)]
pub(crate) struct HaltPanel {
    reason: HaltReason,
    message: SharedString,
    actions: Rc<dyn Actions>,
}

impl HaltPanel {
    pub(crate) fn new(
        reason: &HaltReason,
        message: impl Into<SharedString>,
        actions: &Rc<dyn Actions>,
    ) -> Self {
        Self {
            reason: reason.clone(),
            message: message.into(),
            actions: Rc::clone(actions),
        }
    }

    fn title(theme: &Theme) -> impl IntoElement {
        let destructive = theme.palette(Surface::Window).destructive();
        div()
            .flex()
            .items_center()
            .gap(px(GAP_SM))
            .text_color(destructive)
            .text_size(ScaledText::Lg.size(theme))
            .font_weight(FontWeight::SEMIBOLD)
            .child(
                Icon::new(IconName::WarningCircle)
                    .weight(IconWeight::Fill)
                    .size(px(TITLE_ICON))
                    .color(destructive),
            )
            .child("Crowbar could not start")
    }

    fn buttons(&self) -> impl IntoElement {
        let commands = [Command::OpenLog, Command::Quit];
        div()
            .flex()
            .justify_end()
            .gap(px(GAP_SM))
            .w_full()
            .children(
                commands
                    .into_iter()
                    .map(|command| command_button(command, Command::Quit, &self.actions)),
            )
    }
}

fn command_button(
    command: Command,
    primary: Command,
    actions: &Rc<dyn Actions>,
) -> Button {
    let actions = Rc::clone(actions);
    Button::new(command.id(), command.label())
        .icon(command.icon())
        .size(ButtonSize::Md)
        .variant(command.variant(primary))
        .on_click(move |_, _, cx| actions.perform(command, cx))
}

fn detail_box(
    id: &'static str,
    text: SharedString,
    cx: &App,
) -> impl IntoElement + use<> {
    let palette = Theme::of(cx).palette(Surface::Window);
    div()
        .id(id)
        .w_full()
        .max_h(px(DETAIL_MAX_HEIGHT))
        .overflow_y_scroll()
        .px(px(DETAIL_PADDING_X))
        .py(px(DETAIL_PADDING_Y))
        .rounded(radius::MD)
        .border_1()
        .border_color(palette.border())
        .bg(palette.muted())
        .font_family(Theme::of(cx).mono_family())
        .text_size(px(DETAIL_TEXT_SIZE))
        .line_height(px(DETAIL_TEXT_SIZE * DETAIL_LINE_HEIGHT_RATIO))
        .text_color(palette.foreground())
        .child(text)
}

impl RenderOnce for HaltPanel {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let destructive = palette.destructive();
        let reason = self
            .reason
            .code()
            .map(|code| Badge::new(code.to_owned()).variant(BadgeVariant::Destructive));
        let hint = div()
            .text_size(ScaledText::Xs.size(&theme))
            .text_color(palette.muted_foreground())
            .child("Quit Crowbar, fix the cause above, and open it again.");
        div()
            .flex()
            .flex_col()
            .items_start()
            .gap(px(GAP_MD))
            .w(px(WIDTH))
            .p(px(CARD_PADDING))
            .rounded(radius::XXL)
            .border_1()
            .border_color(destructive.opacity(TINT_BORDER))
            .bg(destructive.opacity(TINT))
            .text_size(ScaledText::Sm.size(&theme))
            .text_color(palette.foreground())
            .child(Self::title(&theme))
            .children(reason)
            .child(self.reason.explanation())
            .child(detail_box("startup-halt-message", self.message.clone(), cx))
            .child(hint)
            .child(self.buttons())
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::{RecordingActions, measure};

    fn panel(code: Option<&str>) -> HaltPanel {
        let actions: Rc<dyn Actions> = Rc::new(RecordingActions::default());
        HaltPanel::new(&HaltReason::from_code(code), "step 3 failed", &actions)
    }

    #[gpui::test]
    fn the_card_has_a_fixed_width_and_a_known_code_adds_a_reason_badge(cx: &mut TestAppContext) {
        let coded = measure(cx, |_, _| panel(Some("layout_newer")));
        let uncoded = measure(cx, |_, _| panel(None));
        assert_eq!(f32::from(coded.size.width), WIDTH);
        assert!(coded.size.height > uncoded.size.height + px(10.0));
    }

    fn measured_box(
        cx: &mut TestAppContext,
        text: String,
    ) -> f32 {
        let boxed = move |_: &mut Window, cx: &mut App| {
            div()
                .w(px(400.0))
                .child(detail_box("box", text.clone().into(), cx))
        };
        f32::from(measure(cx, boxed).size.height)
    }

    #[gpui::test]
    fn the_detail_box_hugs_a_short_message_and_stops_growing_at_its_maximum(
        cx: &mut TestAppContext
    ) {
        let short = measured_box(cx, "step failed".to_owned());
        let long = measured_box(cx, "line\n".repeat(80));
        assert!(short < DETAIL_MAX_HEIGHT, "{short}");
        assert_eq!(long, DETAIL_MAX_HEIGHT);
    }
}
