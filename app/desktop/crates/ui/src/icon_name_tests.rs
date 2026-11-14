use std::collections::HashSet;

use super::*;

#[test]
fn stems_are_unique_kebab_case() {
    let stems: HashSet<_> = IconName::ALL.iter().map(|name| name.stem()).collect();
    assert_eq!(stems.len(), IconName::ALL.len());
    assert!(stems.iter().all(|stem| {
        stem.chars()
            .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
    }));
}

#[test]
fn all_lists_known_icons() {
    assert!(IconName::ALL.contains(&IconName::Check));
    assert_eq!(IconName::Check.stem(), "check");
}
