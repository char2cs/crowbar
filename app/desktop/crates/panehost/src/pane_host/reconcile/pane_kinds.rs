use protocol::PaneId;

use crate::catalog::PaneKind;

pub trait PaneKinds: std::fmt::Debug {
    fn kind_of(
        &self,
        pane: &PaneId,
    ) -> Option<PaneKind>;
}

impl PaneKinds for () {
    fn kind_of(
        &self,
        _pane: &PaneId,
    ) -> Option<PaneKind> {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn with_no_answer_from_the_shell_a_pane_s_type_decides() {
        let pane = PaneId::new("p1").unwrap();
        assert_eq!(().kind_of(&pane), None);
    }
}
