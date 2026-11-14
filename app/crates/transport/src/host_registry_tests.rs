use tempfile::TempDir;

use super::*;

fn id(text: &str) -> HostId {
    HostId::new(text).unwrap()
}

fn at(text: &str) -> Endpoint {
    Endpoint::parse(text).unwrap()
}

fn scratch() -> (TempDir, PathBuf) {
    let dir = TempDir::new().unwrap();
    let path = dir.path().join("hosts.json");
    (dir, path)
}

#[test]
fn insert_get_remove_round_trip() {
    let registry = HostRegistry::in_memory();
    registry
        .insert_blocking(id("host-1"), at("1.2.3.4:3737"))
        .unwrap();
    assert_eq!(registry.get(&id("host-1")), Some(at("1.2.3.4:3737")));
    registry.remove_blocking(&id("host-1")).unwrap();
    assert_eq!(registry.get(&id("host-1")), None);
    assert_eq!(registry.list().len(), 0);
}

#[test]
fn ids_the_router_would_send_to_the_local_daemon_are_refused() {
    let registry = HostRegistry::in_memory();
    for text in ["local", "localhost"] {
        assert!(
            registry.insert_blocking(id(text), at("1.2.3.4:1")).is_err(),
            "{text}"
        );
    }
    assert_eq!(registry.list().len(), 0);
}

#[test]
fn hosts_survive_a_restart() {
    let (_dir, path) = scratch();
    let first = HostRegistry::open(&path).unwrap();
    first
        .insert_blocking(id("host-2"), at("[::1]:4000"))
        .unwrap();
    first
        .insert_blocking(id("host-1"), at("a.example:3737"))
        .unwrap();
    first.remove_blocking(&id("host-2")).unwrap();

    let second = HostRegistry::open(&path).unwrap();

    assert_eq!(second.list(), vec![(id("host-1"), at("a.example:3737"))]);
}

#[test]
fn a_missing_file_is_an_empty_list() {
    let (_dir, path) = scratch();
    assert_eq!(HostRegistry::open(path).unwrap().list().len(), 0);
}

#[test]
fn a_failed_write_leaves_the_list_as_it_was() {
    let (dir, _) = scratch();
    let blocked = dir.path().join("blocked");
    std::fs::create_dir_all(&blocked).unwrap();
    let registry = HostRegistry::open(blocked.join("hosts.json")).unwrap();
    std::fs::remove_dir_all(&blocked).unwrap();
    std::fs::write(&blocked, "a file where a directory must be").unwrap();

    let result = registry.insert_blocking(id("host-1"), at("1.2.3.4:1"));

    assert!(matches!(result, Err(Error::HostsIo { .. })));
    assert_eq!(registry.list().len(), 0);
}

#[tokio::test]
async fn the_async_writers_persist_through_the_blocking_pool() {
    let (_dir, path) = scratch();
    let registry = Arc::new(HostRegistry::open(&path).unwrap());
    registry
        .insert(id("host-1"), at("1.2.3.4:1"))
        .await
        .unwrap();
    assert_eq!(HostRegistry::open(&path).unwrap().list().len(), 1);
    registry.remove(id("host-1")).await.unwrap();
    assert_eq!(HostRegistry::open(&path).unwrap().list().len(), 0);
}

#[test]
fn readers_are_not_blocked_while_a_write_is_on_disk() {
    let (_dir, path) = scratch();
    let registry = HostRegistry::open(&path).unwrap();
    registry
        .insert_blocking(id("host-1"), at("1.2.3.4:1"))
        .unwrap();
    let writing = registry.writer.lock().unwrap();
    assert_eq!(registry.list().len(), 1, "reads need only the read lock");
    drop(writing);
}
