use super::*;
use crate::test_support::prelude::*;

#[gpui::test]
fn the_files_panel_renders_the_workspace_files(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
    cx.run_until_parked();
    assert!(!sidebar.read_with(cx, |this, _| this.data.files().is_empty()));
}

#[test]
fn a_directory_row_toggles_and_a_file_row_opens() {
    use crate::expansion::Expansion;
    use crate::file_kind::FileKind;
    use crate::file_row::FileRow;
    use crate::git_status::GitStatus;
    let row = |kind: FileKind| FileRow {
        path: "src".into(),
        name: "src".into(),
        depth: 0,
        kind,
        status: GitStatus::Clean,
    };
    assert_ne!(
        entry(row(FileKind::Directory(Expansion::Collapsed))).kind,
        entry(row(FileKind::File)).kind
    );
}
