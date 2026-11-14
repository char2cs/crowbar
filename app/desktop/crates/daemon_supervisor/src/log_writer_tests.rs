use std::os::unix::fs::PermissionsExt;

use super::*;

fn mode(path: &Path) -> u32 {
    std::fs::metadata(path).unwrap().permissions().mode() & 0o777
}

#[test]
fn text_is_appended_and_the_file_is_private_to_the_user() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    let mut writer = LogWriter::open(&path, 1 << 20).unwrap();
    writer.write("one\n").unwrap();
    writer.write("two\n").unwrap();
    assert_eq!(std::fs::read_to_string(&path).unwrap(), "one\ntwo\n");
    assert_eq!(mode(&path), 0o600);
}

#[test]
fn an_existing_log_with_wider_permissions_is_narrowed() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    std::fs::write(&path, b"old\n").unwrap();
    std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o644)).unwrap();
    LogWriter::open(&path, 1 << 20).unwrap();
    assert_eq!(mode(&path), 0o600);
}

#[test]
fn a_full_log_is_rotated_aside_when_it_is_opened() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    std::fs::write(&path, b"0123456789ABCDEF").unwrap();
    LogWriter::open(&path, 10)
        .unwrap()
        .write("fresh\n")
        .unwrap();
    assert_eq!(
        std::fs::read(dir.path().join("daemon.log.1")).unwrap(),
        b"0123456789ABCDEF"
    );
    assert_eq!(std::fs::read_to_string(&path).unwrap(), "fresh\n");
}

#[test]
fn a_log_below_the_cap_is_appended_to_in_place() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    std::fs::write(&path, b"short\n").unwrap();
    LogWriter::open(&path, 100)
        .unwrap()
        .write("more\n")
        .unwrap();
    assert!(!dir.path().join("daemon.log.1").exists());
    assert_eq!(std::fs::read_to_string(&path).unwrap(), "short\nmore\n");
}

#[test]
fn the_log_rotates_by_size_while_it_runs_and_never_passes_the_cap() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    let mut writer = LogWriter::open(&path, 20).unwrap();
    for line in ["aaaaaaaaa\n", "bbbbbbbbb\n", "ccccccccc\n"] {
        writer.write(line).unwrap();
    }
    assert_eq!(
        std::fs::read_to_string(dir.path().join("daemon.log.1")).unwrap(),
        "aaaaaaaaa\nbbbbbbbbb\n"
    );
    assert_eq!(std::fs::read_to_string(&path).unwrap(), "ccccccccc\n");
}

#[test]
fn one_entry_larger_than_the_cap_is_still_written() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("daemon.log");
    let mut writer = LogWriter::open(&path, 5).unwrap();
    writer.write("a very long entry\n").unwrap();
    assert_eq!(
        std::fs::read_to_string(&path).unwrap(),
        "a very long entry\n"
    );
}
