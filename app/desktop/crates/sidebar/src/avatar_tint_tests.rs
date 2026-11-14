use super::*;

#[test]
fn every_tint_is_opaque_and_distinct() {
    let all = [
        AvatarTint::Rose,
        AvatarTint::Amber,
        AvatarTint::Emerald,
        AvatarTint::Cyan,
        AvatarTint::Indigo,
        AvatarTint::Violet,
        AvatarTint::Slate,
        AvatarTint::Pink,
    ];
    for (index, tint) in all.iter().enumerate() {
        assert!(
            (tint.color().a - 1.0).abs() < f32::EPSILON,
            "{tint:?} is opaque"
        );
        for other in all.iter().skip(index + 1) {
            assert_ne!(
                tint.color(),
                other.color(),
                "{tint:?} differs from {other:?}"
            );
        }
    }
}
