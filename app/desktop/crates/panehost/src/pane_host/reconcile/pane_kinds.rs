use protocol::PaneId;

use crate::catalog::PaneKind;

/// The shell's answer to who draws each pane. A page and a native view can take turns on one
/// pane (an editor's rich and source views), so the answer is per pane, not per type.
pub trait PaneKinds: std::fmt::Debug {
    /// How `pane` is drawn now, or `None` to let its type decide.
    fn kind_of(
        &self,
        pane: &PaneId,
    ) -> Option<PaneKind>;
}

/// No answer for any pane: every pane's type decides.
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
