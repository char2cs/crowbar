use gpui::Entity;

pub(crate) enum PanePhase {
    Waiting,
    Joining,
    Reading,
    Read(super::landed::Landed),
    Shown(Entity<editorview::EditorView>),
    Unavailable(editor::OpenFailure),
    Retrying(editor::OpenFailure),
}

impl PanePhase {
    pub(crate) fn says_why(&self) -> bool {
        matches!(self, Self::Unavailable(_) | Self::Retrying(_))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_pane_whose_file_could_not_be_read_says_why() {
        let gone = || editor::OpenFailure::of("a.ts", 404, None);
        let cases = [
            (PanePhase::Waiting, false),
            (PanePhase::Reading, false),
            (
                PanePhase::Read(super::super::landed::Landed::default()),
                false,
            ),
            (PanePhase::Unavailable(gone()), true),
            (PanePhase::Retrying(gone()), true),
        ];
        for (phase, expected) in cases {
            assert_eq!(phase.says_why(), expected);
        }
    }
}
