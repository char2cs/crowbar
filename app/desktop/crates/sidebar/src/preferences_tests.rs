use super::*;

fn scratch(name: &str) -> std::path::PathBuf {
    std::env::temp_dir().join(format!("sidebar-prefs-{}-{name}.json", std::process::id()))
}

#[test]
fn a_missing_file_yields_the_defaults() {
    let loaded = Preferences::load(&scratch("missing")).expect("missing file is not an error");
    assert_eq!(loaded, Preferences::default(), "defaults");
}

#[test]
fn saving_and_loading_round_trips_every_field() {
    let path = scratch("roundtrip");
    let mut tree = TreeState::default();
    tree.toggle_fold(&RowId::from("row"));
    let mut panel = Panel::default();
    panel.commit(400.0);
    panel.dock_to(DockSide::Right);
    let mut prefs = Preferences::default().updated(&panel, Mode::Files, &tree);
    prefs
        .last_project_by_host
        .insert("local".into(), "p1".into());
    prefs.save(&path).expect("save");
    let loaded = Preferences::load(&path).expect("load");
    std::fs::remove_file(&path).expect("cleanup");
    assert_eq!(loaded, prefs, "round trip");
    assert_eq!(loaded.panel().side(), DockSide::Right, "side");
    assert!((loaded.panel().preferred() - 400.0).abs() < 0.01, "width");
    assert_eq!(loaded.tree().folded().count(), 1, "folds");
}

#[test]
fn a_corrupt_file_is_an_error_not_a_reset() {
    let path = scratch("corrupt");
    std::fs::write(&path, "not json").expect("write");
    let result = Preferences::load(&path);
    std::fs::remove_file(&path).expect("cleanup");
    assert!(
        matches!(result, Err(Error::ParsePreferences { .. })),
        "parse error surfaces"
    );
}

#[test]
fn partial_files_fall_back_per_field_and_widths_are_clamped() {
    let path = scratch("partial");
    std::fs::write(&path, r#"{"width": 9000.0}"#).expect("write");
    let loaded = Preferences::load(&path).expect("load");
    std::fs::remove_file(&path).expect("cleanup");
    assert_eq!(loaded.mode, Mode::Chats, "default mode");
    assert!(
        (loaded.panel().preferred() - 640.0).abs() < 0.01,
        "clamped to the maximum"
    );
}
