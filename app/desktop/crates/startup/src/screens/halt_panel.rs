use std::rc::Rc;

use gpui::{
    App, FontWeight, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px,
};
use ui::{Badge, BadgeVariant, Icon, IconName, IconWeight, ScaledText, Surface, Theme, radius};

use crate::actions::Actions;
use crate::command::Command;
use crate::machine::HaltReason;
use crate::screens::command_button::CommandButton;
use crate::screens::detail_box::DetailBox;
use crate::screens::spacing::{CARD_PADDING, GAP_MD, GAP_SM, TITLE_ICON};

const WIDTH: f32 = 480.0;
const TINT: f32 = 0.1;
const TINT_BORDER: f32 = 0.2;

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
}

impl HaltPanel {
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
                    .map(|command| CommandButton::new(command, Command::Quit, &self.actions)),
            )
    }
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
            .child(DetailBox::new("startup-halt-message", self.message.clone()))
            .child(hint)
            .child(self.buttons())
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{TestAppContext, px};

    use super::*;
    use crate::test_support::{RecordingActions, measure};

    fn panel(code: Option<&str>) -> HaltPanel {
        let actions: Rc<dyn Actions> = Rc::new(RecordingActions::default());
        HaltPanel::new(&HaltReason::from_code(code), "step 3 failed", &actions)
    }

    #[gpui::test]
    fn the_card_has_a_fixed_width(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| panel(Some("step_failed")));
        assert_eq!(f32::from(bounds.size.width), WIDTH);
    }

    #[gpui::test]
    fn a_known_code_adds_a_reason_badge(cx: &mut TestAppContext) {
        let coded = measure(cx, |_, _| panel(Some("layout_newer")));
        let uncoded = measure(cx, |_, _| panel(None));
        assert!(coded.size.height > uncoded.size.height + px(10.0));
    }
}
