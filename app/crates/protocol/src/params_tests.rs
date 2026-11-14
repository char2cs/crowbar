use super::*;

#[test]
fn serializes_as_a_plain_object_in_key_order() {
    let params = Params::default()
        .with("threadId", "t1")
        .unwrap()
        .with("spaceId", "s1")
        .unwrap();
    assert_eq!(
        serde_json::to_string(&params).unwrap(),
        r#"{"spaceId":"s1","threadId":"t1"}"#,
        "key order"
    );
    assert_eq!(params.get("spaceId"), Some("s1"), "lookup");
    assert_eq!(params.iter().count(), 2, "iteration");
    assert!(!params.is_empty(), "non-empty");
    assert!(Params::default().is_empty(), "empty");
}

#[test]
fn rejects_non_string_values() {
    assert!(
        serde_json::from_str::<Params>(r#"{"a":1}"#).is_err(),
        "number value"
    );
}

#[test]
fn keys_and_values_stay_inside_their_alphabets() {
    for (key, value) in [
        ("", "v"),
        ("a b", "v"),
        ("k.k", "v"),
        ("k", ""),
        ("k", "a b"),
        ("k", "a/b"),
    ] {
        assert!(
            Params::default().with(key, value).is_err(),
            "{key:?}={value:?}"
        );
    }
    let long = "k".repeat(65);
    assert!(Params::default().with(long, "v").is_err(), "long key");
    assert!(
        Params::default().with("k", "v".repeat(129)).is_err(),
        "long value"
    );
}

#[test]
fn entry_count_is_capped_on_construction_and_on_the_wire() {
    let mut params = Params::default();
    for index in 0..MAX_PARAMS {
        params = params.with(format!("k{index}"), "v").unwrap();
    }
    assert!(
        params.clone().with("one-too-many", "v").is_err(),
        "over the cap"
    );
    assert!(
        params.with("k0", "other").is_ok(),
        "replacing is not growing"
    );
    let wire: serde_json::Map<String, serde_json::Value> = (0..=MAX_PARAMS)
        .map(|index| (format!("k{index}"), "v".into()))
        .collect();
    assert!(
        serde_json::from_value::<Params>(wire.into()).is_err(),
        "wire object over the cap"
    );
    assert!(
        serde_json::from_str::<Params>(r#"{"bad key":"v"}"#).is_err(),
        "wire key validated"
    );
}
