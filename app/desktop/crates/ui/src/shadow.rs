//! Box shadows of the spec (Tailwind defaults and the custom ones).

use gpui::{BoxShadow, Hsla, hsla, point, px};

fn black(alpha: f32) -> Hsla {
    hsla(0.0, 0.0, 0.0, alpha)
}

fn layer(
    y: f32,
    blur: f32,
    spread: f32,
    alpha: f32,
) -> BoxShadow {
    BoxShadow {
        color: black(alpha),
        offset: point(px(0.0), px(y)),
        blur_radius: px(blur),
        spread_radius: px(spread),
    }
}

type Offsets = (f32, f32, f32);

const XS: &[Offsets] = &[(1.0, 2.0, 0.0)];
const SM: &[Offsets] = &[(1.0, 3.0, 0.0), (1.0, 2.0, -1.0)];
const MD: &[Offsets] = &[(4.0, 6.0, -1.0), (2.0, 4.0, -2.0)];
const LG: &[Offsets] = &[(10.0, 15.0, -3.0), (4.0, 6.0, -4.0)];
const DROPDOWN: &[Offsets] = &[(14.0, 30.0, -24.0)];
const DROPDOWN_ALPHA: f32 = 0.45;
const HANDLE: &[Offsets] = &[(0.0, 5.0, 1.0)];

fn layers(
    table: &[Offsets],
    alpha: f32,
) -> Vec<BoxShadow> {
    table
        .iter()
        .map(|&(y, blur, spread)| layer(y, blur, spread, alpha))
        .collect()
}

/// `shadow-xs` at the given black alpha (default 5% via `shadow-xs/5`).
pub fn xs(alpha: f32) -> Vec<BoxShadow> {
    layers(XS, alpha)
}

/// `shadow-sm` with every layer at `alpha` (0.1 plain, 0.05 for `/5`).
pub fn sm(alpha: f32) -> Vec<BoxShadow> {
    layers(SM, alpha)
}

/// `shadow-md` with every layer at `alpha`.
pub fn md(alpha: f32) -> Vec<BoxShadow> {
    layers(MD, alpha)
}

/// `shadow-lg` with every layer at `alpha`.
pub fn lg(alpha: f32) -> Vec<BoxShadow> {
    layers(LG, alpha)
}

/// `shadow-xs` tinted with `color`, as buttons do with `shadow-primary/24`.
pub fn xs_tinted(color: Hsla) -> Vec<BoxShadow> {
    layers(XS, 0.0)
        .into_iter()
        .map(|shadow| BoxShadow { color, ..shadow })
        .collect()
}

/// The custom dropdown shadow `0 14px 30px -24px rgba(0,0,0,.45)`.
pub fn dropdown() -> Vec<BoxShadow> {
    layers(DROPDOWN, DROPDOWN_ALPHA)
}

/// The pane handle shadow `0 0 5px 1px rgba(0,0,0,.1)`.
pub fn handle() -> Vec<BoxShadow> {
    layers(HANDLE, crate::opacity::PERCENT_10)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn geometry(shadows: &[BoxShadow]) -> Vec<(f32, f32, f32, f32)> {
        shadows
            .iter()
            .map(|s| {
                (
                    s.offset.y.into(),
                    s.blur_radius.into(),
                    s.spread_radius.into(),
                    s.color.a,
                )
            })
            .collect()
    }

    type Layers = &'static [(f32, f32, f32, f32)];

    type Case = (&'static str, fn() -> Vec<BoxShadow>, Layers);

    const CASES: &[Case] = &[
        ("shadow-xs", || xs(0.05), &[(1.0, 2.0, 0.0, 0.05)]),
        (
            "shadow-sm",
            || sm(0.1),
            &[(1.0, 3.0, 0.0, 0.1), (1.0, 2.0, -1.0, 0.1)],
        ),
        (
            "shadow-md",
            || md(0.1),
            &[(4.0, 6.0, -1.0, 0.1), (2.0, 4.0, -2.0, 0.1)],
        ),
        (
            "shadow-lg",
            || lg(0.05),
            &[(10.0, 15.0, -3.0, 0.05), (4.0, 6.0, -4.0, 0.05)],
        ),
        ("dropdown", dropdown, &[(14.0, 30.0, -24.0, 0.45)]),
        ("handle", handle, &[(0.0, 5.0, 1.0, 0.1)]),
    ];

    #[test]
    fn geometry_matches_the_spec() {
        for &(name, build, expected) in CASES {
            assert_eq!(geometry(&build()), expected, "{name}");
        }
    }

    #[test]
    fn tinted_keeps_the_geometry_of_xs() {
        let color = hsla(0.3, 0.5, 0.5, 0.24);
        let shadows = xs_tinted(color);
        assert_eq!(geometry(&shadows), [(1.0, 2.0, 0.0, 0.24)]);
    }
}
