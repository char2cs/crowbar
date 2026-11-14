use super::*;

#[test]
fn the_owner_row_is_exposed() {
    let row = RowId::from("r");
    assert_eq!(Editing::Nothing.row(), None, "idle");
    for editing in [
        Editing::Renaming(row.clone()),
        Editing::Branching(row.clone()),
        Editing::Drafting(row.clone()),
    ] {
        assert_eq!(editing.row(), Some(&row), "{editing:?}");
    }
}
