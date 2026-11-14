use super::*;
use crate::command::Command;
use crate::row_id::RowId;

#[test]
fn constructors_set_the_tone() {
    let intent = Intent::Send(Command::Lock(RowId::from("r")));
    let normal = MenuEntry::action("Lock", IconName::Lock, intent.clone());
    let destructive = MenuEntry::destructive("Delete", IconName::Trash, intent);
    assert!(
        matches!(
            normal,
            MenuEntry::Action {
                tone: Tone::Normal,
                ..
            }
        ),
        "normal"
    );
    assert!(
        matches!(
            destructive,
            MenuEntry::Action {
                tone: Tone::Destructive,
                ..
            }
        ),
        "destructive"
    );
}
