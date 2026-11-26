use protocol::PaneId;

use crate::panes::tracking::dirty_panes::DirtyPanes;
use crate::shell::dialogs::unsaved::close_choice::CloseChoice;
use crate::shell::dialogs::unsaved::close_step::CloseStep;
use crate::shell::dialogs::unsaved::closing::Closing;

#[derive(Debug, PartialEq)]
pub(crate) struct PendingClose {
    closing: Closing,
    dirty: Vec<PaneId>,
    saving: bool,
}

impl PendingClose {
    pub(crate) fn new(
        closing: Closing,
        dirty: Vec<PaneId>,
    ) -> Self {
        Self {
            closing,
            dirty,
            saving: false,
        }
    }

    pub(crate) fn dirty(&self) -> &[PaneId] {
        &self.dirty
    }

    pub(crate) fn choose(
        mut self,
        choice: CloseChoice,
        flags: &DirtyPanes,
    ) -> CloseStep<Self> {
        match choice {
            CloseChoice::Cancel => CloseStep::Stop,
            CloseChoice::Discard => CloseStep::Run(self.closing),
            CloseChoice::Save => {
                self.saving = true;
                self.settle(flags)
            },
        }
    }

    pub(crate) fn settle(
        self,
        flags: &DirtyPanes,
    ) -> CloseStep<Self> {
        let unsaved = self.dirty.iter().any(|pane| flags.contains(pane));
        if self.saving && !unsaved {
            return CloseStep::Run(self.closing);
        }
        CloseStep::Wait(self)
    }
}

#[cfg(test)]
mod tests {
    use layout::{Command, TabId};

    use super::*;

    fn pane(id: &str) -> PaneId {
        PaneId::new(id).unwrap()
    }

    fn asked(flags: &DirtyPanes) -> PendingClose {
        flags.mark(pane("a"));
        let closing = Closing::Panes(vec![Command::CloseTab { tab: TabId(1) }]);
        PendingClose::new(closing, vec![pane("a")])
    }

    #[derive(Debug, PartialEq)]
    enum Outcome {
        Closed,
        Waiting,
        Dropped,
    }

    fn outcome(step: &CloseStep<PendingClose>) -> Outcome {
        match step {
            CloseStep::Run(_) => Outcome::Closed,
            CloseStep::Wait(_) => Outcome::Waiting,
            CloseStep::Stop => Outcome::Dropped,
        }
    }

    struct Case {
        name: &'static str,
        choice: CloseChoice,
        still_dirty: bool,
        expected: Outcome,
    }

    const CASES: [Case; 4] = [
        Case {
            name: "cancel",
            choice: CloseChoice::Cancel,
            still_dirty: true,
            expected: Outcome::Dropped,
        },
        Case {
            name: "discard",
            choice: CloseChoice::Discard,
            still_dirty: true,
            expected: Outcome::Closed,
        },
        Case {
            name: "save with work unsaved",
            choice: CloseChoice::Save,
            still_dirty: true,
            expected: Outcome::Waiting,
        },
        Case {
            name: "save of a pane already clean",
            choice: CloseChoice::Save,
            still_dirty: false,
            expected: Outcome::Closed,
        },
    ];

    fn answered(case: &Case) -> CloseStep<PendingClose> {
        let flags = DirtyPanes::default();
        let question = asked(&flags);
        if !case.still_dirty {
            flags.clear(&pane("a"));
        }
        question.choose(case.choice, &flags)
    }

    #[test]
    fn each_answer_closes_waits_or_drops_the_close() {
        for case in CASES {
            assert_eq!(outcome(&answered(&case)), case.expected, "{}", case.name);
        }
    }

    #[test]
    fn a_saving_close_waits_for_clean_panes_and_an_unanswered_question_never_closes() {
        let flags = DirtyPanes::default();
        let CloseStep::Wait(saving) = asked(&flags).choose(CloseChoice::Save, &flags) else {
            panic!("a dirty pane must be saved first");
        };
        let CloseStep::Wait(saving) = saving.settle(&flags) else {
            panic!("a failed save keeps the pane open");
        };
        flags.clear(&pane("a"));
        assert_eq!(outcome(&saving.settle(&flags)), Outcome::Closed);
        let unanswered = asked(&flags);
        flags.clear(&pane("a"));
        assert_eq!(outcome(&unanswered.settle(&flags)), Outcome::Waiting);
    }
}
