//! A row standing for a running or failed daemon operation.

use gpui::{
    AnyElement, App, Div, ElementId, InteractiveElement, IntoElement, ParentElement, Pixels,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Icon, IconName, IconWeight, Palette, Spinner, Surface, Theme};

use crate::command::Command;
use crate::intent::Intent;
use crate::operation_phase::OperationPhase;
use crate::row::Row;
use crate::row_action::{RowAction, RowHandler};
use crate::row_report::report;
use crate::row_shell::row_shell;
use crate::scale::{ICON_LARGE, ICON_MEDIUM, ICON_SMALL, SPACE_SM, TEXT_BODY, TEXT_SMALL};

const LABEL_MAX_WIDTH: Pixels = px(96.0);

const DISABLED_OPACITY: f32 = 0.6;

#[derive(IntoElement)]
pub(crate) struct OperationRow {
    pub(crate) row: Row,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for OperationRow {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let Some(operation) = self.row.operation.clone() else {
            return div().into_any_element();
        };
        let id = ElementId::from(SharedString::from(format!(
            "operation-{}",
            self.row.id.as_str()
        )));
        let tail = match &operation.phase {
            OperationPhase::Running => None,
            OperationPhase::Failed { reason } => {
                Some(failure(&self.row, reason, &self.handler, palette))
            },
        };
        row_shell(id, self.row.depth, palette)
            .opacity(DISABLED_OPACITY)
            .child(leading(&self.row, &operation.phase, palette))
            .child(label(operation.label, Theme::of(cx).mono_family(), palette))
            .children(tail)
            .into_any_element()
    }
}

fn leading(
    row: &Row,
    phase: &OperationPhase,
    palette: &Palette,
) -> AnyElement {
    match phase {
        OperationPhase::Running => div()
            .flex()
            .flex_none()
            .size(ICON_LARGE)
            .items_center()
            .justify_center()
            .child(
                Spinner::new(SharedString::from(format!("op-{}", row.id.as_str())))
                    .size(ICON_MEDIUM),
            )
            .into_any_element(),
        OperationPhase::Failed { .. } => div()
            .flex_none()
            .w(ICON_LARGE)
            .text_size(TEXT_SMALL)
            .text_color(palette.destructive())
            .child("✕")
            .into_any_element(),
    }
}

fn label(
    text: String,
    mono: &'static str,
    palette: &Palette,
) -> Div {
    div()
        .flex_1()
        .min_w_0()
        .truncate()
        .font_family(mono)
        .text_size(TEXT_BODY)
        .text_color(palette.muted_foreground())
        .child(text)
}

fn failure(
    row: &Row,
    reason: &str,
    handler: &RowHandler,
    palette: &'static Palette,
) -> Div {
    let dismiss = RowAction::Intent(Intent::Send(Command::Dismiss(row.id.clone())));
    let id = ElementId::from(SharedString::from(format!("dismiss-{}", row.id.as_str())));
    let button = div()
        .id(id)
        .cursor_pointer()
        .text_color(palette.muted_foreground())
        .hover(move |style| style.text_color(palette.foreground()))
        .child(
            Icon::new(IconName::X)
                .weight(IconWeight::Bold)
                .size(ICON_SMALL),
        )
        .on_click(report(handler, dismiss));
    div()
        .flex()
        .flex_none()
        .items_center()
        .gap(SPACE_SM)
        .text_size(TEXT_SMALL)
        .child(div().text_color(palette.destructive()).child("failed"))
        .child(
            div()
                .max_w(LABEL_MAX_WIDTH)
                .truncate()
                .text_color(palette.muted_foreground())
                .child(reason.to_owned()),
        )
        .child(button)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use std::rc::Rc;

    use super::*;
    use crate::operation::Operation;
    use crate::test_support::{chat, measure, recording_handler};

    fn row_with(phase: OperationPhase) -> Row {
        let mut row = chat("x");
        row.operation = Some(Operation {
            label: "New thread".into(),
            phase,
        });
        row
    }

    #[gpui::test]
    fn a_running_operation_is_one_row_tall(cx: &mut TestAppContext) {
        let (handler, _) = recording_handler();
        let bounds = measure(cx, move |_, _| OperationRow {
            row: row_with(OperationPhase::Running),
            handler: Rc::clone(&handler),
        });
        assert_eq!(f32::from(bounds.size.height), 38.0);
    }

    #[gpui::test]
    fn a_row_without_an_operation_draws_nothing(cx: &mut TestAppContext) {
        let (handler, _) = recording_handler();
        let bounds = measure(cx, move |_, _| OperationRow {
            row: chat("x"),
            handler: Rc::clone(&handler),
        });
        assert_eq!(f32::from(bounds.size.height), 0.0);
    }
}
