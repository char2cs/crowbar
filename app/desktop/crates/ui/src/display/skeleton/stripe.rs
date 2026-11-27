use std::ops::Range;

use gpui::Pixels;

pub(crate) struct Stripe {
    pub(crate) core: Pixels,
    pub(crate) reach: Pixels,
}

impl Stripe {
    pub(crate) fn rises(&self) -> Range<Pixels> {
        self.core - self.reach..self.core
    }

    pub(crate) fn falls(&self) -> Range<Pixels> {
        self.core..self.core + self.reach
    }
}

#[cfg(test)]
mod tests {
    use gpui::px;

    use super::*;

    #[test]
    fn the_two_halves_meet_at_the_core_and_match_in_width() {
        let stripe = Stripe {
            core: px(500.0),
            reach: px(120.0),
        };
        assert_eq!(stripe.rises(), px(380.0)..px(500.0));
        assert_eq!(stripe.falls(), px(500.0)..px(620.0));
    }
}
