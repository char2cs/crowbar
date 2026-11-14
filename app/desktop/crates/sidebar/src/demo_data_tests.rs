use super::*;

#[test]
fn renaming_to_a_taken_name_is_refused_and_leaves_the_tree_alone() {
    let data = DemoData::reference();
    let refused = data.dispatch(Command::Rename {
        row: RowId::from("chat"),
        name: "main".into(),
    });
    assert!(
        matches!(refused, Err(Error::Refused { .. })),
        "got {refused:?}"
    );
    let tree = data.tree(&ProjectId::from("inv"));
    assert!(
        tree.rows.iter().any(|row| row.label == "Untitled chat"),
        "label kept"
    );
}

#[test]
fn committing_a_removal_drops_the_row() {
    let data = DemoData::reference();
    data.dispatch(Command::CommitRemoval(RowId::from("chat")))
        .expect("accepted");
    assert!(
        data.tree(&ProjectId::from("inv"))
            .rows
            .iter()
            .all(|row| row.id != RowId::from("chat"))
    );
}

#[test]
fn the_rich_data_lists_three_spaces_and_two_hosts() {
    let data = DemoData::rich();
    assert_eq!(data.projects().len(), 3);
    assert_eq!(data.hosts().len(), 2);
}
