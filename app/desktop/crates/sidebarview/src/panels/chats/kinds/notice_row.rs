//! The unread notice above a space's tree.

use gpui::{App, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};
use sidebar::{Command, Intent, Notice, ProjectId};
use ui::{Button, ButtonSize, ButtonVariant, Icon, IconName, Surface, Theme};

use crate::sidebar::{ICON_MEDIUM, SPACE_LG, SPACE_MD, SPACE_SM, TEXT_SMALL};
use crate::sidebar::{RowAction, RowHandler};

#[derive(IntoElement)]
pub(crate) struct NoticeRow {
    pub(crate) notice: Notice,
    pub(crate) project: ProjectId,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for NoticeRow {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let (project, handler) = (self.project, self.handler);
        div()
            .flex()
            .gap(SPACE_MD)
            .px(SPACE_LG)
            .py(SPACE_SM)
            .text_size(TEXT_SMALL)
            .text_color(palette.muted_foreground())
            .child(
                div().mt(px(2.0)).child(
                    Icon::new(IconName::WarningCircle)
                        .size(ICON_MEDIUM)
                        .color(palette.warning()),
                ),
            )
            .child(div().flex_1().child(self.notice.text()))
            .child(
                Button::new("notice-retry", "Retry")
                    .variant(ButtonVariant::Ghost)
                    .size(ButtonSize::Xs)
                    .on(Surface::Chrome)
                    .on_click(move |_, window, cx| {
                        let intent = Intent::Send(Command::RetryChats(project.clone()));
                        handler(RowAction::Intent(intent), window, cx);
                    }),
            )
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use std::rc::Rc;

    use super::*;
    use crate::test_support::{measure, recording_handler};

    #[gpui::test]
    fn the_notice_renders_its_sentence_above_a_retry_button(cx: &mut TestAppContext) {
        let (handler, _) = recording_handler();
        let bounds = measure(cx, move |_, _| NoticeRow {
            notice: Notice::Project,
            project: ProjectId::from("inv"),
            handler: Rc::clone(&handler),
        });
        assert!(f32::from(bounds.size.height) > 0.0);
    }
}
