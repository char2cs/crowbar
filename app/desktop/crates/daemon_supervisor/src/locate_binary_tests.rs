use super::*;

fn dir(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("cb-loc-{tag}-{}", std::process::id()));
    std::fs::create_dir_all(&dir).unwrap();
    dir
}

#[test]
fn an_explicit_path_wins_without_being_checked() {
    let found = locate_binary(Some(Path::new("/x/crowbar-api")), None, None).unwrap();
    assert_eq!(found, PathBuf::from("/x/crowbar-api"));
}

#[test]
fn the_binary_beside_the_app_beats_one_on_the_path() {
    let app = dir("beside");
    let other = dir("path");
    std::fs::write(app.join("crowbar-api"), b"").unwrap();
    std::fs::write(other.join("crowbar-api"), b"").unwrap();
    let exe = app.join("crowbar");
    let found = locate_binary(None, Some(&exe), Some(other.as_os_str())).unwrap();
    assert_eq!(found, app.join("crowbar-api"));
}

#[test]
fn the_path_is_searched_and_a_miss_names_every_place_looked() {
    let other = dir("only-path");
    std::fs::write(other.join("crowbar-api"), b"").unwrap();
    assert_eq!(
        locate_binary(None, None, Some(other.as_os_str())).unwrap(),
        other.join("crowbar-api")
    );
    let empty = dir("empty");
    let err =
        locate_binary(None, Some(&empty.join("crowbar")), Some(empty.as_os_str())).unwrap_err();
    assert!(err.to_string().contains("crowbar-api"), "{err}");
}
