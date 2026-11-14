use super::*;

#[test]
fn without_nesting_the_halves_decide() {
    assert_eq!(
        DropModes::REORDER.at(0.49, FolderTarget::No),
        Some(DropMode::Before),
        "upper half"
    );
    assert_eq!(
        DropModes::REORDER.at(0.5, FolderTarget::No),
        Some(DropMode::After),
        "lower half"
    );
}

#[test]
fn only_nesting_always_nests() {
    for ratio in [0.0, 0.5, 1.0] {
        assert_eq!(
            DropModes::INTO.at(ratio, FolderTarget::Yes),
            Some(DropMode::Into),
            "ratio {ratio}"
        );
    }
}

#[test]
fn bands_are_twenty_percent_for_folders_and_thirty_for_the_rest() {
    let cases = [
        (0.19, FolderTarget::Yes, DropMode::Before),
        (0.21, FolderTarget::Yes, DropMode::Into),
        (0.81, FolderTarget::Yes, DropMode::After),
        (0.29, FolderTarget::No, DropMode::Before),
        (0.31, FolderTarget::No, DropMode::Into),
        (0.71, FolderTarget::No, DropMode::After),
    ];
    for (ratio, folder, expected) in cases {
        assert_eq!(
            DropModes::ALL.at(ratio, folder),
            Some(expected),
            "ratio {ratio} {folder:?}"
        );
    }
}

#[test]
fn nothing_allowed_means_no_drop() {
    assert_eq!(DropModes::NONE.at(0.5, FolderTarget::No), None, "refused");
}
