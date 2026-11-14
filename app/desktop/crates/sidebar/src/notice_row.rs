//! The unread notice above a space's tree.

use gpui::{App, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px};
use ui::{Button, ButtonSize, ButtonVariant, Icon, IconName, Surface, Theme};

use crate::command::Command;
use crate::intent::Intent;
use crate::notice::Notice;
use crate::project_id::ProjectId;
use crate::row_action::{RowAction, RowHandler};

#[derive(IntoElement)]
pub(crate) struct NoticeRow {
    pub(crate) notice: Notice,
    pub(crate) project: ProjectId,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for NoticeRow {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let (project, handler) = (self.project, self.handler);
        div()
            .flex()
            .gap(px(8.0))
            .px(px(12.0))
            .py(px(6.0))
            .text_size(px(12.0))
            .text_color(palette.muted_foreground())
            .child(
                div().mt(px(2.0)).child(
                    Icon::new(IconName::WarningCircle)
                        .size(px(14.0))
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
#[path = "notice_row_tests.rs"]
mod tests;
