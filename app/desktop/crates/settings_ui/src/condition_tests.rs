use std::collections::BTreeSet;

use super::*;
use crate::values::Values;

fn holds(condition: &Condition) -> bool {
    let values = Values::default().with("engine", "custom");
    let defaults = Values::default().with("zoom", 100.0);
    let capabilities = BTreeSet::from(["browser.clearData".to_owned()]);
    condition.holds(&Scope {
        values: &values,
        defaults: &defaults,
        capabilities: &capabilities,
    })
}

#[test]
fn leaf_rules() {
    assert!(
        holds(&Condition::equals("engine", "custom")),
        "stored value"
    );
    assert!(holds(&Condition::equals("zoom", 100.0)), "default value");
    assert!(
        !holds(&Condition::equals("engine", "google")),
        "different value"
    );
    assert!(
        holds(&Condition::capability("browser.clearData")),
        "present capability"
    );
    assert!(
        !holds(&Condition::capability("hosts.add")),
        "absent capability"
    );
}

#[test]
fn combinators() {
    let yes = Condition::equals("engine", "custom");
    let no = Condition::capability("nope");
    assert!(
        holds(&Condition::All(vec![yes.clone(), yes.clone()])),
        "all true"
    );
    assert!(
        !holds(&Condition::All(vec![yes.clone(), no.clone()])),
        "one false"
    );
    assert!(holds(&Condition::Any(vec![no.clone(), yes])), "one true");
    assert!(!holds(&Condition::Any(vec![no])), "none true");
    assert!(holds(&Condition::All(vec![])), "empty all");
    assert!(!holds(&Condition::Any(vec![])), "empty any");
}
