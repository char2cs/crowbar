use gpui::Entity;

pub(crate) enum PanePhase {
    /// Its chat is not loaded yet, so there is no daemon to read from.
    Waiting,
    /// The read is out.
    Reading,
    /// The text arrived; the view is made when the pane is first drawn.
    Read(super::landed::Landed),
    /// The document shows.
    Shown(Entity<editorview::EditorView>),
    /// The document could not be read; the pane says why and offers to try again.
    Unavailable(editor::OpenFailure),
    /// It could not be read, and a Try again is reading it once more.
    Retrying(editor::OpenFailure),
}

impl PanePhase {
    pub(super) fn says_why(&self) -> bool {
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
