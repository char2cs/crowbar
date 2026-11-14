use super::*;

#[test]
fn path_resolves_only_embedded_weights() {
    assert_eq!(Icon::path(IconName::Gear, IconWeight::Regular), Some("icons/regular/gear.svg"));
    assert_eq!(Icon::path(IconName::GitBranch, IconWeight::Fill), Some("icons/fill/git-branch.svg"));
    assert_eq!(Icon::path(IconName::Gear, IconWeight::Duotone), None);
}

#[test]
fn every_icon_has_at_least_one_weight() {
    for &name in IconName::ALL {
        let weights = [IconWeight::Regular, IconWeight::Bold, IconWeight::Fill, IconWeight::Duotone];
        assert!(weights.iter().any(|&w| Icon::path(name, w).is_some()), "{name:?}");
    }
}
