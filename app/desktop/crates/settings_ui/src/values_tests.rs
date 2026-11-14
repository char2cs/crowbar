use super::*;

#[test]
fn set_get_and_remove_round_trip() {
    let key = Key::from("tabSize");
    let mut values = Values::default().with("tabSize", 4.0);
    assert_eq!(values.get(&key), Some(&Value::Number(4.0)));
    values.set(key.clone(), Value::Number(8.0));
    assert_eq!(values.get(&key), Some(&Value::Number(8.0)));
    values.remove(&key);
    assert_eq!(values.get(&key), None);
}
