use super::*;

#[test]
fn builders_compose_a_full_record() {
    let record = Record::new("claude", "Claude Code")
        .icon("robot")
        .status(Tone::Success)
        .line(Line::plain("v2.1"))
        .badge(Line::plain("OK").toned(Tone::Success))
        .flag(RecordFlag::new("enabled", "Enable").on())
        .action(RecordAction::new("kill", "skull", "Kill"))
        .movable();
    assert_eq!(record.id(), "claude");
    assert_eq!(record.icon.as_deref(), Some("robot"));
    assert_eq!(record.status, Some(Tone::Success));
    assert_eq!((record.lines.len(), record.flags.len()), (1, 1));
    assert!(record.badge.is_some() && record.action.is_some() && record.movable);
}
