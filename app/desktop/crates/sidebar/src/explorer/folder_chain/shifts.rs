#[derive(Default)]
pub(crate) struct Shifts(Vec<(u8, u8)>);

impl Shifts {
    pub(crate) fn at(
        &mut self,
        depth: u8,
    ) -> u8 {
        self.0.retain(|(deepest, _)| depth > *deepest);
        self.0
            .iter()
            .fold(0_u8, |total, (_, by)| total.saturating_add(*by))
    }

    pub(crate) fn add(
        &mut self,
        (deepest, by): (u8, usize),
    ) {
        let by = u8::try_from(by).unwrap_or(u8::MAX);
        if by > 0 {
            self.0.push((deepest, by));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_shift_applies_below_its_chain_and_ends_at_its_depth() {
        let mut shifts = Shifts::default();
        shifts.add((2, 2));
        assert_eq!(shifts.at(3), 2);
        assert_eq!(shifts.at(2), 0);
        assert_eq!(shifts.at(3), 0);
    }
}
