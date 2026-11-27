use gpui::{Pixels, Size, px};

use crate::display::skeleton::stripe::Stripe;

const TILE_WIDTHS: f32 = 2.0;
const HALF_THICKNESS: f32 = 0.1;
const SIN_120: f32 = 0.866_025_4;
const COS_120: f32 = 0.5;
const TILT: f32 = COS_120 / SIN_120;
const TILES: [f32; 4] = [-2.0, -1.0, 0.0, 1.0];

pub(crate) fn stripes(
    phase: f32,
    viewport: Size<Pixels>,
    row: Pixels,
) -> Vec<Stripe> {
    let width = f32::from(viewport.width);
    let height = f32::from(viewport.height);
    let gradient = TILE_WIDTHS * width * SIN_120 + height * COS_120;
    let reach = px(HALF_THICKNESS * gradient / SIN_120);
    let slant = (f32::from(row) - height / 2.0) * TILT;
    TILES
        .iter()
        .map(|tile| {
            let origin = TILE_WIDTHS * width * (phase + tile);
            px(origin + width - slant)
        })
        .map(|core| Stripe { core, reach })
        .filter(|stripe| {
            stripe.core + stripe.reach > px(0.0) && stripe.core - stripe.reach < viewport.width
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use gpui::size;

    use super::*;

    const VIEWPORT: (f32, f32) = (1000.0, 800.0);

    fn cores(
        phase: f32,
        row: f32,
    ) -> Vec<f32> {
        let viewport = size(px(VIEWPORT.0), px(VIEWPORT.1));
        stripes(phase, viewport, px(row))
            .iter()
            .map(|stripe| f32::from(stripe.core))
            .collect()
    }

    #[test]
    fn the_stripe_crosses_the_viewport_once_per_cycle_and_rests_off_screen() {
        type Case = (f32, &'static [f32]);
        const CASES: &[Case] = &[
            (0.0, &[1000.0]),
            (0.1, &[1200.0]),
            (0.25, &[]),
            (0.5, &[0.0]),
            (0.75, &[500.0]),
            (0.95, &[900.0]),
        ];
        for &(phase, want) in CASES {
            let got = cores(phase, 400.0);
            let same =
                got.len() == want.len() && got.iter().zip(want).all(|(a, b)| (a - b).abs() < 1.0);
            assert!(same, "{phase}: {got:?} vs {want:?}");
        }
    }

    #[test]
    fn rows_below_the_middle_lag_to_the_left_of_the_stripe_core() {
        let (high, low) = (cores(0.75, 100.0), cores(0.75, 700.0));
        let shift = high.first().zip(low.first()).map(|(a, b)| a - b);
        let expected = 600.0 * TILT;
        assert!(
            shift.is_some_and(|shift| (shift - expected).abs() < 1.0),
            "{shift:?}"
        );
    }
}
