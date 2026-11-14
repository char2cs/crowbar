use super::*;

#[test]
fn wire_shape() {
    let migrating = AppPhase::Migrating {
        state: "chats".into(),
        progress: Some(0.5),
    };
    assert_eq!(
        serde_json::to_string(&migrating).unwrap(),
        r#"{"phase":"migrating","state":"chats","progress":0.5}"#,
        "migrating"
    );
    assert_eq!(
        serde_json::to_string(&AppPhase::Ready).unwrap(),
        r#"{"phase":"ready"}"#,
        "ready"
    );
    let down: AppPhase =
        serde_json::from_str(r#"{"phase":"daemonUnavailable","reason":"x"}"#).unwrap();
    assert_eq!(
        down,
        AppPhase::DaemonUnavailable { reason: "x".into() },
        "unavailable"
    );
}

#[test]
fn only_ready_shows_the_layout() {
    let migrating = AppPhase::Migrating {
        state: String::new(),
        progress: None,
    };
    assert!(!migrating.shows_layout(), "migrating");
    assert!(!AppPhase::Starting.shows_layout(), "starting");
    assert!(
        !AppPhase::DaemonUnavailable {
            reason: String::new()
        }
        .shows_layout(),
        "down"
    );
    assert!(AppPhase::Ready.shows_layout(), "ready");
}
