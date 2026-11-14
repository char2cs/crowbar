use std::rc::Rc;

use gpui::{
    App, FontWeight, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px,
};
use ui::{Badge, BadgeVariant, Icon, IconName, IconWeight, Surface, Theme, UiText, radius};

use crate::command_button::CommandButton;
use crate::detail_box::DetailBox;
use crate::halt_reason::HaltReason;
use crate::startup_actions::StartupActions;
use crate::startup_command::StartupCommand;

const WIDTH: f32 = 480.0;
const TINT: f32 = 0.1;
const TINT_BORDER: f32 = 0.2;

#[derive(IntoElement)]
pub(crate) struct HaltPanel {
    reason: HaltReason,
    message: SharedString,
    actions: Rc<dyn StartupActions>,
}

impl HaltPanel {
    pub(crate) fn new(
        reason: &HaltReason,
        message: impl Into<SharedString>,
        actions: &Rc<dyn StartupActions>,
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
            .gap(px(8.0))
            .text_color(destructive)
            .text_size(UiText::Lg.size(theme))
            .font_weight(FontWeight::SEMIBOLD)
            .child(
                Icon::new(IconName::WarningCircle)
                    .weight(IconWeight::Fill)
                    .size(px(20.0))
                    .color(destructive),
            )
            .child("Crowbar could not start")
    }

    fn buttons(&self) -> impl IntoElement {
        let commands = [StartupCommand::OpenLog, StartupCommand::Quit];
        div().flex().justify_end().gap(px(8.0)).w_full().children(
            commands
                .into_iter()
                .map(|command| CommandButton::new(command, StartupCommand::Quit, &self.actions)),
        )
    }
}

impl RenderOnce for HaltPanel {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let destructive = palette.destructive();
        let reason = self
            .reason
            .code()
            .map(|code| Badge::new(code.to_owned()).variant(BadgeVariant::Destructive));
        let hint = div()
            .text_size(UiText::Xs.size(&theme))
            .text_color(palette.muted_foreground())
            .child("Quit Crowbar, fix the cause above, and open it again.");
        div()
            .flex()
            .flex_col()
            .items_start()
            .gap(px(12.0))
            .w(px(WIDTH))
            .p(px(20.0))
            .rounded(radius::XXL)
            .border_1()
            .border_color(destructive.opacity(TINT_BORDER))
            .bg(destructive.opacity(TINT))
            .text_size(UiText::Sm.size(&theme))
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
#[path = "halt_panel_tests.rs"]
mod tests;
