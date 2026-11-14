use super::*;
use crate::demo_data::DemoData;
use crate::row_id::RowId;

#[test]
fn a_sink_may_refuse_a_command_with_a_reason() {
    let data = DemoData::reference();
    let refused = data.dispatch(Command::Rename {
        row: RowId::from("chat"),
        name: "main".into(),
    });
    assert!(matches!(refused, Err(Error::Refused { .. })));
}

#[test]
fn a_sink_accepts_a_valid_command() {
    let data = DemoData::reference();
    assert!(data.dispatch(Command::OpenRow(RowId::from("chat"))).is_ok());
}
