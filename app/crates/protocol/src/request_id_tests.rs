use super::*;

#[test]
fn is_a_plain_number_on_the_wire() {
    let id = RequestId::new(7);
    assert_eq!(id.get(), 7, "number");
    assert_eq!(serde_json::to_string(&id).unwrap(), "7", "serialize");
    assert_eq!(
        serde_json::from_str::<RequestId>("7").unwrap(),
        id,
        "deserialize"
    );
}
