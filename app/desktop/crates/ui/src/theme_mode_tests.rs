use super::*;

#[test]
fn resolve_follows_system_only_in_system_mode() {
    const CASES: &[(ThemeMode, Appearance, Appearance)] = &[
        (ThemeMode::Light, Appearance::Dark, Appearance::Light),
        (ThemeMode::Dark, Appearance::Light, Appearance::Dark),
        (ThemeMode::System, Appearance::Dark, Appearance::Dark),
        (ThemeMode::System, Appearance::Light, Appearance::Light),
    ];
    for &(mode, system, expected) in CASES {
        assert_eq!(mode.resolve(system), expected, "{mode:?} on {system:?}");
    }
}
