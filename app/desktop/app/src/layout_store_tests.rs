use model::{Command, TabId};

use super::*;

fn layout() -> Layout {
    let mut layout = Layout::default();
    layout
        .apply(Command::AddTab {
            id: TabId(1),
            title: "Chat".into(),
        })
        .unwrap();
    layout
}

#[test]
fn a_first_run_has_an_empty_layout() {
    let dir = tempfile::tempdir().unwrap();
    let store = LayoutStore::new(dir.path().join("layout.json"));
    assert_eq!(store.load().unwrap().tabs().len(), 0, "empty");
}

#[test]
fn a_saved_layout_loads_back_and_leaves_no_staging_file() {
    let dir = tempfile::tempdir().unwrap();
    let store = LayoutStore::new(dir.path().join("state").join("layout.json"));
    store.save(&layout()).unwrap();
    assert_eq!(store.load().unwrap(), layout(), "round trip");
    let leftovers: Vec<_> = std::fs::read_dir(dir.path().join("state"))
        .unwrap()
        .map(|entry| entry.unwrap().file_name())
        .collect();
    assert_eq!(leftovers, ["layout.json"], "only the document remains");
}

#[test]
fn a_corrupt_file_is_an_error_not_an_empty_layout() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("layout.json");
    std::fs::write(&path, "{ not json").unwrap();
    assert!(
        matches!(LayoutStore::new(path).load(), Err(Error::Parse(..))),
        "refused"
    );
}
