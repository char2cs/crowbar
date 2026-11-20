//! What the search did to one block of a page.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Slot {
    Plain,
    Shown,
    Hidden,
}

impl Slot {
    pub(crate) fn section(self) -> Option<Self> {
        (self != Self::Plain).then_some(self)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_sections_report_themselves() {
        assert_eq!(Slot::Plain.section(), None);
        assert_eq!(Slot::Hidden.section(), Some(Slot::Hidden));
    }
}
