use super::*;

fn geometry(shadows: &[BoxShadow]) -> Vec<(f32, f32, f32, f32)> {
    shadows
        .iter()
        .map(|s| (s.offset.y.into(), s.blur_radius.into(), s.spread_radius.into(), s.color.a))
        .collect()
}

#[test]
fn tailwind_geometry_matches_the_spec() {
    assert_eq!(geometry(&xs(0.05)), [(1.0, 2.0, 0.0, 0.05)]);
    assert_eq!(geometry(&sm(0.1)), [(1.0, 3.0, 0.0, 0.1), (1.0, 2.0, -1.0, 0.1)]);
    assert_eq!(geometry(&md(0.1)), [(4.0, 6.0, -1.0, 0.1), (2.0, 4.0, -2.0, 0.1)]);
    assert_eq!(geometry(&lg(0.05)), [(10.0, 15.0, -3.0, 0.05), (4.0, 6.0, -4.0, 0.05)]);
}

#[test]
fn custom_shadows_match_the_spec() {
    assert_eq!(geometry(&dropdown()), [(14.0, 30.0, -24.0, 0.45)]);
    assert_eq!(geometry(&handle()), [(0.0, 5.0, 1.0, 0.1)]);
}

#[test]
fn tinted_keeps_the_geometry_of_xs() {
    let color = hsla(0.3, 0.5, 0.5, 0.24);
    let shadows = xs_tinted(color);
    assert_eq!(geometry(&shadows), [(1.0, 2.0, 0.0, 0.24)]);
}
