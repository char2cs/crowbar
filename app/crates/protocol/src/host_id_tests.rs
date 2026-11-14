use super::*;
use crate::error::Error;

#[test]
fn accepts_local_and_paired_hosts() {
    assert!(HostId::new("local").is_ok(), "local");
    assert!(HostId::new("host-7_b").is_ok(), "paired");
}

#[test]
fn rejects_authority_breaking_text() {
    assert!(
        matches!(
            HostId::new("a/b"),
            Err(Error::InvalidIdCharacter { found: '/', .. })
        ),
        "slash"
    );
    assert!(HostId::new("Upper").is_err(), "upper case");
    assert!(HostId::new("").is_err(), "empty");
}

#[test]
fn display_parse_and_serde_round_trip() {
    let id: HostId = "local".parse().unwrap();
    assert_eq!(id.to_string(), "local", "display");
    assert_eq!(id.as_ref(), "local", "as_ref");
    assert_eq!(
        serde_json::to_string(&id).unwrap(),
        "\"local\"",
        "serialize"
    );
    assert!(
        serde_json::from_str::<HostId>("\"A\"").is_err(),
        "deserialize validates"
    );
    assert_eq!(String::from(id), "local", "into string");
}
