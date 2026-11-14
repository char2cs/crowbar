use super::*;
use crate::test_support::prelude::*;

#[gpui::test]
fn the_changes_panel_renders_the_changed_files(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update(cx, |this, cx| this.set_mode(Mode::Changes, cx));
    cx.run_until_parked();
    assert_eq!(
        sidebar.read_with(cx, |this, _| this.data.changes().files.len()),
        1
    );
}

#[test]
fn a_changed_file_row_carries_its_line_counts() {
    use crate::changed_file::ChangedFile;
    use crate::git_status::GitStatus;
    let file = ChangedFile {
        path: "a/b.rs".into(),
        status: GitStatus::Added,
        added: 3,
        deleted: 1,
    };
    assert_eq!(entry(file).counts, Some((3, 1)));
}
