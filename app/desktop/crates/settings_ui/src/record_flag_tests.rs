use super::*;

#[test]
fn keeps_its_fields() {
    let flag = RecordFlag::new("enabled", "Enable Claude Code").on();
    assert_eq!(
        (flag.id.as_str(), flag.label.as_str(), flag.on),
        ("enabled", "Enable Claude Code", true)
    );
}
