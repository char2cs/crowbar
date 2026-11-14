use std::rc::Rc;

use gpui::{
    App, ClickEvent, ClipboardItem, FontWeight, IntoElement, ParentElement, RenderOnce, Styled,
    Window, div, px,
};
use ui::{
    Button, ButtonSize, ButtonVariant, Icon, IconName, IconWeight, Spinner, Surface, Theme, UiText,
    radius, shadow,
};

use crate::command_button::CommandButton;
use crate::detail_box::DetailBox;
use crate::outage::Outage;
use crate::startup_actions::StartupActions;
use crate::startup_command::StartupCommand;

const WIDTH: f32 = 560.0;

#[derive(IntoElement)]
pub(crate) struct OutagePanel {
    outage: Outage,
    actions: Rc<dyn StartupActions>,
}

impl OutagePanel {
    pub(crate) fn new(outage: &Outage, actions: &Rc<dyn StartupActions>) -> Self {
        Self {
            outage: outage.clone(),
            actions: Rc::clone(actions),
        }
    }

    fn title(&self, theme: &Theme) -> impl IntoElement {
        let palette = theme.palette(Surface::Window);
        let lead = if self.outage.is_restarting() {
            Spinner::new("startup-outage-spinner")
                .size(px(18.0))
                .into_any_element()
        } else {
            Icon::new(IconName::LinkBreak)
                .weight(IconWeight::Regular)
                .size(px(20.0))
                .color(palette.warning())
                .into_any_element()
        };
        div()
            .flex()
            .items_center()
            .gap(px(10.0))
            .text_size(UiText::Lg.size(theme))
            .font_weight(FontWeight::SEMIBOLD)
            .child(lead)
            .child(self.outage.headline())
    }

    fn copy_button(&self) -> Option<Button> {
        let Outage::GaveUp { report } = &self.outage else {
            return None;
        };
        let report = report.clone();
        Some(
            Button::new("startup-copy", "Copy details")
                .icon(IconName::Copy)
                .size(ButtonSize::Sm)
                .variant(ButtonVariant::Ghost)
                .on_click(copy_to_clipboard(report)),
        )
    }

    fn buttons(&self) -> impl IntoElement {
        let primary = if self.outage.is_retryable() {
            StartupCommand::Retry
        } else {
            StartupCommand::OpenLog
        };
        let commands = [
            self.outage.is_retryable().then_some(StartupCommand::Retry),
            Some(StartupCommand::OpenLog),
            self.outage.is_retryable().then_some(StartupCommand::Quit),
        ];
        div().flex().gap(px(8.0)).children(
            commands
                .into_iter()
                .flatten()
                .map(|command| CommandButton::new(command, primary, &self.actions)),
        )
    }
}

fn copy_to_clipboard(text: String) -> impl Fn(&ClickEvent, &mut Window, &mut App) {
    move |_, _, cx| cx.write_to_clipboard(ClipboardItem::new_string(text.clone()))
}

impl RenderOnce for OutagePanel {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let detail = self
            .outage
            .detail()
            .map(|text| DetailBox::new("startup-outage-detail", text.to_owned()));
        let footer = div()
            .flex()
            .flex_row_reverse()
            .justify_between()
            .items_center()
            .w_full()
            .child(self.buttons())
            .children(self.copy_button());
        div()
            .flex()
            .flex_col()
            .items_start()
            .gap(px(12.0))
            .w(px(WIDTH))
            .p(px(20.0))
            .rounded(radius::XXL)
            .border_1()
            .border_color(palette.border())
            .bg(palette.popover())
            .shadow(shadow::lg(0.05))
            .text_size(UiText::Sm.size(&theme))
            .text_color(palette.foreground())
            .child(self.title(&theme))
            .child(
                div()
                    .text_color(palette.muted_foreground())
                    .child(self.outage.explanation()),
            )
            .children(detail)
            .child(footer)
    }
}

#[cfg(test)]
#[path = "outage_panel_tests.rs"]
mod tests;
