//! A row standing for a running or failed daemon operation.

use gpui::{
    AnyElement, App, Div, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::font::MONO_FAMILY;
use ui::{Icon, IconName, IconWeight, Palette, Spinner, Surface, Theme};

use crate::command::Command;
use crate::intent::Intent;
use crate::operation_phase::OperationPhase;
use crate::row::Row;
use crate::row_action::{RowAction, RowHandler};
use crate::row_report::report;
use crate::row_shell::row_shell;

const DISABLED_OPACITY: f32 = 0.6;

#[derive(IntoElement)]
pub(crate) struct OperationRow {
    pub(crate) row: Row,
    pub(crate) handler: RowHandler,
}

impl RenderOnce for OperationRow {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
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
            .child(label(operation.label, palette))
            .children(tail)
            .into_any_element()
    }
}

fn leading(row: &Row, phase: &OperationPhase, palette: &Palette) -> AnyElement {
    match phase {
        OperationPhase::Running => div()
            .flex()
            .flex_none()
            .size(px(16.0))
            .items_center()
            .justify_center()
            .child(
                Spinner::new(SharedString::from(format!("op-{}", row.id.as_str()))).size(px(14.0)),
            )
            .into_any_element(),
        OperationPhase::Failed { .. } => div()
            .flex_none()
            .w(px(16.0))
            .text_size(px(12.0))
            .text_color(palette.destructive())
            .child("✕")
            .into_any_element(),
    }
}

fn label(text: String, palette: &Palette) -> Div {
    div()
        .flex_1()
        .min_w_0()
        .truncate()
        .font_family(MONO_FAMILY)
        .text_size(px(13.0))
        .text_color(palette.muted_foreground())
        .child(text)
}

fn failure(row: &Row, reason: &str, handler: &RowHandler, palette: &'static Palette) -> Div {
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
                .size(px(12.0)),
        )
        .on_click(report(handler, dismiss));
    div()
        .flex()
        .flex_none()
        .items_center()
        .gap(px(6.0))
        .text_size(px(12.0))
        .child(div().text_color(palette.destructive()).child("failed"))
        .child(
            div()
                .max_w(px(96.0))
                .truncate()
                .text_color(palette.muted_foreground())
                .child(reason.to_owned()),
        )
        .child(button)
}

#[cfg(test)]
#[path = "operation_row_tests.rs"]
mod tests;
