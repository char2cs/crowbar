use std::collections::BTreeSet;
use std::fs;
use std::path::Path;

use super::*;

#[test]
fn load_returns_embedded_svg_and_none_for_unknown() {
    let svg = Assets.load("icons/regular/gear.svg").expect("load").expect("embedded");
    assert!(svg.starts_with(b"<svg"));
    assert!(Assets.load("icons/regular/nope.svg").expect("load").is_none());
}

#[test]
fn list_filters_by_prefix() {
    let bold = Assets.list("icons/bold/").expect("list");
    assert!(!bold.is_empty());
    assert!(bold.iter().all(|name| name.starts_with("icons/bold/")));
}

#[test]
fn every_icon_name_has_a_regular_or_other_weight_and_no_file_is_orphaned() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("assets/icons");
    let mut on_disk = BTreeSet::new();
    for weight in fs::read_dir(&root).expect("icons dir") {
        let weight = weight.expect("entry").path();
        for file in fs::read_dir(&weight).expect("weight dir") {
            let name = file.expect("entry").file_name().into_string().expect("utf8");
            let folder = weight.file_name().expect("folder").to_string_lossy().into_owned();
            on_disk.insert(format!("icons/{folder}/{name}"));
        }
    }
    let embedded: BTreeSet<String> = FILES.iter().map(|file| file.path.to_owned()).collect();
    assert_eq!(embedded, on_disk, "embedded table and assets/icons differ");
    let stems: BTreeSet<&str> = crate::icon_name::IconName::ALL.iter().map(|icon| icon.stem()).collect();
    for stem in stems {
        let found = embedded.iter().any(|name| name.ends_with(&format!("/{stem}.svg")));
        assert!(found, "icon {stem} has no embedded weight");
    }
}
