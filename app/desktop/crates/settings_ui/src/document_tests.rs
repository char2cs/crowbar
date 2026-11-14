use super::*;
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::number_range::NumberRange;

fn size_row() -> Row {
    Row::new(
        "tabSize",
        "Tab Size",
        Control::number(NumberRange::new(1.0, 8.0, 1.0), ControlWidth::NumberCompact),
    )
    .default_value(2.0)
}

fn document() -> Document {
    Document::new(Values::default(), Values::default().with("tabSize", 2.0))
}

#[test]
fn defaults_show_until_a_value_is_stored() {
    let mut document = document();
    let row = size_row();
    assert_eq!(document.value(&row.key), Some(&Value::Number(2.0)));
    assert!(document.values().get(&row.key).is_none());
    assert!(document.is_default(&row));
    document.apply(&row.key, Value::Number(4.0));
    assert!(!document.is_default(&row));
    assert_eq!(document.text(&row), "4");
}

#[test]
fn a_failed_write_restores_what_was_stored_before() {
    let mut document = document();
    let key = Key::from("tabSize");
    document.apply(&key, Value::Number(4.0));
    document.apply(&key, Value::Number(5.0));
    assert!(document.has_unsaved());
    document.fail(&key);
    assert!(
        document.values().get(&key).is_none(),
        "both writes are undone to the original state"
    );
    assert!(!document.has_unsaved());
    document.apply(&key, Value::Number(6.0));
    document.acknowledge(&key);
    document.apply(&key, Value::Number(7.0));
    document.fail(&key);
    assert_eq!(
        document.values().get(&key),
        Some(&Value::Number(6.0)),
        "back to the acknowledged value"
    );
}

#[test]
fn remote_changes_do_not_override_a_write_in_flight() {
    let mut document = document();
    let key = Key::from("tabSize");
    document.apply(&key, Value::Number(4.0));
    document.remote(&key, Value::Number(8.0));
    assert_eq!(document.values().get(&key), Some(&Value::Number(4.0)));
    document.acknowledge(&key);
    document.remote(&key, Value::Number(8.0));
    assert_eq!(document.values().get(&key), Some(&Value::Number(8.0)));
}

#[test]
fn drafts_track_dirtiness_until_reverted_or_applied() {
    let mut document = document();
    let row = size_row();
    document.edit(&row.key, "5");
    assert!(document.is_dirty(&row));
    assert_eq!(document.text(&row), "5");
    assert_eq!(document.draft(&row.key), Some("5"));
    document.edit(&row.key, "2");
    assert!(
        !document.is_dirty(&row),
        "typing the stored value back is not dirty"
    );
    document.revert(&row.key);
    assert_eq!(document.draft(&row.key), None);
    document.edit(&row.key, "7");
    document.apply(&row.key, Value::Number(7.0));
    assert_eq!(document.draft(&row.key), None, "applying clears the draft");
    assert_eq!(document.values().get(&row.key), Some(&Value::Number(7.0)));
    assert_eq!(document.defaults().get(&row.key), Some(&Value::Number(2.0)));
}
