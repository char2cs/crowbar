//! Box shadows of the spec (Tailwind defaults and the custom ones).

use gpui::{BoxShadow, Hsla, hsla, point, px};

fn black(alpha: f32) -> Hsla {
    hsla(0.0, 0.0, 0.0, alpha)
}

fn layer(y: f32, blur: f32, spread: f32, alpha: f32) -> BoxShadow {
    BoxShadow {
        color: black(alpha),
        offset: point(px(0.0), px(y)),
        blur_radius: px(blur),
        spread_radius: px(spread),
    }
}

/// `shadow-xs` at the given black alpha (default 5% via `shadow-xs/5`).
pub fn xs(alpha: f32) -> Vec<BoxShadow> {
    vec![layer(1.0, 2.0, 0.0, alpha)]
}

/// `shadow-sm` with every layer at `alpha` (0.1 plain, 0.05 for `/5`).
pub fn sm(alpha: f32) -> Vec<BoxShadow> {
    vec![layer(1.0, 3.0, 0.0, alpha), layer(1.0, 2.0, -1.0, alpha)]
}

/// `shadow-md` with every layer at `alpha`.
pub fn md(alpha: f32) -> Vec<BoxShadow> {
    vec![layer(4.0, 6.0, -1.0, alpha), layer(2.0, 4.0, -2.0, alpha)]
}

/// `shadow-lg` with every layer at `alpha`.
pub fn lg(alpha: f32) -> Vec<BoxShadow> {
    vec![layer(10.0, 15.0, -3.0, alpha), layer(4.0, 6.0, -4.0, alpha)]
}

/// `shadow-xs` tinted with `color`, as buttons do with `shadow-primary/24`.
pub fn xs_tinted(color: Hsla) -> Vec<BoxShadow> {
    vec![BoxShadow {
        color,
        ..layer(1.0, 2.0, 0.0, 0.0)
    }]
}

/// The custom dropdown shadow `0 14px 30px -24px rgba(0,0,0,.45)`.
pub fn dropdown() -> Vec<BoxShadow> {
    vec![layer(14.0, 30.0, -24.0, 0.45)]
}

/// The pane handle shadow `0 0 5px 1px rgba(0,0,0,.1)`.
pub fn handle() -> Vec<BoxShadow> {
    vec![layer(0.0, 5.0, 1.0, 0.1)]
}

#[cfg(test)]
#[path = "shadow_tests.rs"]
mod tests;
