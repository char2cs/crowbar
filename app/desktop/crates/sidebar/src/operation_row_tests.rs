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
