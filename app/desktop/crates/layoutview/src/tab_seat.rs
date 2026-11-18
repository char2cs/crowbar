use protocol::PaneId;

#[derive(Clone, Copy)]
pub(crate) struct TabSeat<'a> {
    pub(crate) pane: &'a PaneId,
    pub(crate) index: usize,
}

impl<'a> TabSeat<'a> {
    pub(crate) const fn at(
        pane: &'a PaneId,
        index: usize,
    ) -> Self {
        Self { pane, index }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_seat_names_the_pane_and_the_position_in_its_strip() {
        let pane = PaneId::new("a").unwrap();
        let seat = TabSeat::at(&pane, 3);
        assert_eq!((seat.pane.as_str(), seat.index), ("a", 3));
    }
}
