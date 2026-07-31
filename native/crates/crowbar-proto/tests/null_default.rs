//! Direct tests for `null_to_default`, the one function in `crowbar-proto`
//! that is code rather than a shape.
//!
//! The generated round-trip suite already drives it through every non-optional
//! container field in the crate, but only on the two paths a well-formed daemon
//! response takes: `null` for a nil Go slice or map, and a real array or object
//! otherwise. This file exists for the third path, which no well-formed response
//! reaches and which is the one where a plausible-looking helper goes wrong.
//!
//! The failure mode being pinned: a helper written as
//! `Option::<T>::deserialize(d).unwrap_or_default()` — dropping the `?` — passes
//! every test in the generated suite and is catastrophically wrong. It turns a
//! *malformed* payload into an empty collection, so a daemon that started
//! sending `{"results": [1, 2]}` where strings belong would present as "no
//! results" for as long as it took someone to notice, instead of failing the
//! response at the point of the mistake. `null` meaning "empty" is a
//! deliberate accommodation of `encoding/json`; a type error meaning "empty" is
//! data loss.
//!
//! Written by hand rather than generated because it is a property of the helper
//! itself, and it must keep holding whatever the Go handlers do next. It is
//! also the only test here that is not derived from the daemon, so it does not
//! move when the daemon does.

use std::collections::HashMap;

use crowbar_proto::null_default::null_to_default;

/// A nil Go slice or map marshals as JSON `null`; serde refuses `null` for
/// `Vec`/`HashMap`, so the helper maps it onto the empty collection.
#[test]
fn null_becomes_the_default() {
    let mut de = serde_json::Deserializer::from_str("null");
    let out: Vec<String> = null_to_default(&mut de).expect("null is accepted");
    assert_eq!(out, Vec::<String>::new());

    let mut de = serde_json::Deserializer::from_str("null");
    let out: HashMap<String, i64> = null_to_default(&mut de).expect("null is accepted");
    assert_eq!(out, HashMap::new());

    // Not only containers: the helper is generic over `T: Default`, and the
    // enum fallback and scalar cases go through the same code.
    let mut de = serde_json::Deserializer::from_str("null");
    let out: String = null_to_default(&mut de).expect("null is accepted");
    assert_eq!(out, String::new());
}

/// A present value passes through untouched — the helper is a coercion for one
/// specific wire quirk, not a filter.
#[test]
fn a_present_value_passes_through() {
    let mut de = serde_json::Deserializer::from_str(r#"["a", "b"]"#);
    let out: Vec<String> = null_to_default(&mut de).expect("an array is accepted");
    assert_eq!(out, vec![String::from("a"), String::from("b")]);

    let mut de = serde_json::Deserializer::from_str(r#"{"k": 7}"#);
    let out: HashMap<String, i64> = null_to_default(&mut de).expect("an object is accepted");
    assert_eq!(out, HashMap::from([(String::from("k"), 7)]));

    // An empty array is already the default value, and must still arrive as
    // itself rather than through the null path — otherwise the two would be
    // indistinguishable and the test above would prove nothing.
    let mut de = serde_json::Deserializer::from_str("[]");
    let out: Vec<String> = null_to_default(&mut de).expect("an empty array is accepted");
    assert_eq!(out, Vec::<String>::new());
}

/// A malformed payload is an error, not an empty collection. This is the whole
/// reason the helper deserialises `Option<T>` and propagates with `?` instead
/// of swallowing the result.
#[test]
fn a_type_error_is_not_swallowed() {
    let mut de = serde_json::Deserializer::from_str("[1, 2]");
    let out: Result<Vec<String>, _> = null_to_default(&mut de);
    let err = out.expect_err("an array of numbers is not an array of strings");
    assert!(
        err.to_string().contains("invalid type"),
        "expected a type error, got {err}",
    );

    let mut de = serde_json::Deserializer::from_str("42");
    let out: Result<Vec<String>, _> = null_to_default(&mut de);
    assert!(out.is_err(), "a number is not an array");

    let mut de = serde_json::Deserializer::from_str(r#"{"k": "not a number"}"#);
    let out: Result<HashMap<String, i64>, _> = null_to_default(&mut de);
    assert!(out.is_err(), "a string is not an i64");
}
