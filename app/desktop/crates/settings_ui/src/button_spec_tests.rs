use super::*;

#[test]
fn builders_fill_the_optional_parts() {
    let spec = ButtonSpec::new("Clear data", Action::backend("browser.clearData"))
        .icon("trash")
        .confirm(Confirm::new("Clear?", "Sure?", "Clear data"))
        .enabled_when(Condition::capability("browser.clearData"))
        .disabled_note("Not available");
    assert_eq!(spec.icon.as_deref(), Some("trash"));
    assert!(spec.confirm.is_some() && spec.enabled.is_some());
    assert_eq!(spec.disabled_note.as_deref(), Some("Not available"));
}
