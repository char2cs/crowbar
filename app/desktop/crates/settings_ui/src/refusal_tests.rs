use super::*;

#[test]
fn every_refusal_explains_itself() {
    for refusal in [Refusal::BareKey, Refusal::UnsupportedModifier] {
        assert!(!refusal.message().is_empty(), "{refusal:?}");
    }
}
