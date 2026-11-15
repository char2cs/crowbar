use sidebar::{Command, RowId};

use super::*;

#[test]
fn opening_a_thread_opens_its_chat() {
    let action = action_for(&Command::Thread(RowId::from("t1"))).unwrap();
    assert_eq!(
        action,
        SidebarAction::Open(chat_type().unwrap(), chat_params("t1").unwrap()),
        "chat of the thread"
    );
}

#[test]
fn settings_open_the_modal_and_data_commands_are_unserved() {
    assert_eq!(
        action_for(&Command::OpenSettings).unwrap(),
        SidebarAction::OpenSettings,
        "settings"
    );
    assert_eq!(
        action_for(&Command::ImportRepo).unwrap(),
        SidebarAction::Unserved,
        "import"
    );
}
