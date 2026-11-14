use super::*;

fn id(text: &str) -> HostId {
    HostId::new(text).unwrap()
}

fn at(text: &str) -> Endpoint {
    Endpoint::parse(text).unwrap()
}

fn scratch(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("cb-reg-{tag}-{}", std::process::id()));
    std::fs::create_dir_all(&dir).unwrap();
    dir.join("hosts.json")
}

#[test]
fn insert_get_remove_round_trip() {
    let registry = HostRegistry::in_memory();
    registry.insert(id("host-1"), at("1.2.3.4:3737")).unwrap();
    assert_eq!(registry.get(&id("host-1")), Some(at("1.2.3.4:3737")));
    registry.remove(&id("host-1")).unwrap();
    assert_eq!(registry.get(&id("host-1")), None);
    assert_eq!(registry.list().len(), 0);
}

#[test]
fn ids_the_router_would_not_send_to_that_host_are_refused() {
    let registry = HostRegistry::in_memory();
    for text in ["local", "localhost", "host_1"] {
        assert!(
            registry.insert(id(text), at("1.2.3.4:1")).is_err(),
            "{text}"
        );
    }
    assert_eq!(registry.list().len(), 0);
}

#[test]
fn hosts_survive_a_restart() {
    let path = scratch("restart");
    let first = HostRegistry::open(&path).unwrap();
    first.insert(id("host-2"), at("[::1]:4000")).unwrap();
    first.insert(id("host-1"), at("a.example:3737")).unwrap();
    first.remove(&id("host-2")).unwrap();

    let second = HostRegistry::open(&path).unwrap();

    assert_eq!(second.list(), vec![(id("host-1"), at("a.example:3737"))]);
}

#[test]
fn a_missing_file_is_an_empty_list() {
    assert_eq!(HostRegistry::open(scratch("missing")).unwrap().list().len(), 0);
}

#[test]
fn a_failed_write_leaves_the_list_as_it_was() {
    let path = scratch("unwritable");
    let dir = path.parent().unwrap().join("blocked");
    std::fs::create_dir_all(&dir).unwrap();
    let registry = HostRegistry::open(dir.join("hosts.json")).unwrap();
    std::fs::remove_dir_all(&dir).unwrap();
    std::fs::write(&dir, "a file where a directory must be").unwrap();

    let result = registry.insert(id("host-1"), at("1.2.3.4:1"));

    assert!(matches!(result, Err(Error::HostsIo { .. })));
    assert_eq!(registry.list().len(), 0);
}
