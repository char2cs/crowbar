use std::rc::Rc;

use gpui::{
    AnyElement, App, ClickEvent, ClipboardItem, FontWeight, Hsla, IntoElement, ParentElement,
    RenderOnce, Styled, Window, div, px,
};
use ui::{
    Button, ButtonSize, ButtonVariant, Icon, IconName, IconWeight, Spinner, Surface, Theme, UiText,
    radius, shadow,
};

use crate::actions::Actions;
use crate::command::Command;
use crate::machine::Outage;
use crate::screens::command_button::CommandButton;
use crate::screens::detail_box::DetailBox;
use crate::screens::spacing::{CARD_PADDING, GAP_MD, GAP_SM, TITLE_ICON};

const WIDTH: f32 = 560.0;
const SPINNER_SIZE: f32 = 18.0;
const TITLE_GAP: f32 = 10.0;
const SHADOW_OPACITY: f32 = 0.05;

#[derive(IntoElement)]
pub(crate) struct OutagePanel {
    outage: Outage,
    actions: Rc<dyn Actions>,
}

impl OutagePanel {
    pub(crate) fn new(
        outage: &Outage,
        actions: &Rc<dyn Actions>,
    ) -> Self {
        Self {
            outage: outage.clone(),
            actions: Rc::clone(actions),
        }
    }

    fn title(
        &self,
        theme: &Theme,
    ) -> impl IntoElement {
        let palette = theme.palette(Surface::Window);
        let lead = self.lead(palette.warning());
        div()
            .flex()
            .items_center()
            .gap(px(TITLE_GAP))
            .text_size(UiText::Lg.size(theme))
            .font_weight(FontWeight::SEMIBOLD)
            .child(lead)
            .child(self.outage.headline())
    }

    fn lead(
        &self,
        warning: Hsla,
    ) -> AnyElement {
        if self.outage.is_restarting() {
            return Spinner::new("startup-outage-spinner")
                .size(px(SPINNER_SIZE))
                .into_any_element();
        }
        Icon::new(IconName::LinkBreak)
            .weight(IconWeight::Regular)
            .size(px(TITLE_ICON))
            .color(warning)
            .into_any_element()
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

    fn primary(&self) -> Command {
        if self.outage.is_retryable() {
            return Command::Retry;
        }
        Command::OpenLog
    }

    fn buttons(&self) -> impl IntoElement {
        let primary = self.primary();
        let commands = [
            self.outage.is_retryable().then_some(Command::Retry),
            Some(Command::OpenLog),
            self.outage.is_retryable().then_some(Command::Quit),
        ];
        div().flex().gap(px(GAP_SM)).children(
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
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
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
            .gap(px(GAP_MD))
            .w(px(WIDTH))
            .p(px(CARD_PADDING))
            .rounded(radius::XXL)
            .border_1()
            .border_color(palette.border())
            .bg(palette.popover())
            .shadow(shadow::lg(SHADOW_OPACITY))
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
mod tests {
    use std::rc::Rc;

    use gpui::{TestAppContext, px};

    use super::*;
    use crate::command::Command;
    use crate::test_support::{RecordingActions, click_across, measure, open};

    fn gave_up() -> Outage {
        Outage::GaveUp {
            report: "crowbar 1.0\nexit status 1".to_owned(),
        }
    }

    fn sink() -> (Rc<RecordingActions>, Rc<dyn Actions>) {
        let recording = Rc::new(RecordingActions::default());
        let shared = Rc::clone(&recording) as Rc<dyn Actions>;
        (recording, shared)
    }

    #[gpui::test]
    fn every_outage_renders_a_card_of_the_fixed_width(cx: &mut TestAppContext) {
        let outages = [
            Outage::Unresponsive,
            Outage::Recovering {
                attempt: 2,
                reason: "exit status 1".to_owned(),
            },
            gave_up(),
            Outage::Stopped,
        ];
        for outage in outages {
            let (_, actions) = sink();
            let bounds = measure(cx, move |_, _| OutagePanel::new(&outage, &actions));
            assert_eq!(f32::from(bounds.size.width), WIDTH);
        }
    }

    #[gpui::test]
    fn the_report_of_a_supervisor_that_gave_up_can_be_copied(cx: &mut TestAppContext) {
        let (_, actions) = sink();
        let with_report = measure(cx, {
            let actions = Rc::clone(&actions);
            move |_, _| OutagePanel::new(&gave_up(), &actions)
        });
        let without = measure(cx, move |_, _| OutagePanel::new(&Outage::Stopped, &actions));
        assert!(with_report.size.height > without.size.height + px(40.0));
    }

    #[gpui::test]
    fn a_stopped_daemon_offers_every_command(cx: &mut TestAppContext) {
        let (recording, actions) = sink();
        let bounds = measure(cx, {
            let actions = Rc::clone(&actions);
            move |_, _| OutagePanel::new(&Outage::Stopped, &actions)
        });
        let cx = open(cx, move |_, _| OutagePanel::new(&Outage::Stopped, &actions));
        click_across(cx, bounds);
        let performed = recording.performed();
        for command in [Command::Retry, Command::OpenLog, Command::Quit] {
            assert!(performed.contains(&command), "{command:?} in {performed:?}");
        }
    }

    #[gpui::test]
    fn a_restarting_daemon_offers_only_the_log(cx: &mut TestAppContext) {
        let (recording, actions) = sink();
        let outage = Outage::Unresponsive;
        let bounds = measure(cx, {
            let (actions, outage) = (Rc::clone(&actions), outage.clone());
            move |_, _| OutagePanel::new(&outage, &actions)
        });
        let cx = open(cx, move |_, _| OutagePanel::new(&outage, &actions));
        click_across(cx, bounds);
        let performed = recording.performed();
        assert!(performed.contains(&Command::OpenLog));
        assert!(performed.iter().all(|command| *command == Command::OpenLog));
    }
}
