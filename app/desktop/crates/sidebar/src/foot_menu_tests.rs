use super::*;
use crate::tone::Tone;

#[test]
fn three_entries_with_the_destructive_one_last() {
    let entries = foot_menu(&ProjectId::from("p"));
    assert_eq!(entries.len(), 3, "import, folder, delete");
    assert!(
        matches!(&entries[2], MenuEntry::Action { label, tone: Tone::Destructive, .. } if label == "Delete space"),
        "delete space is destructive and last"
    );
}
