pub(crate) mod save_key;

use gpui::{KeyBinding, actions};
use gpui_kit::base::input::{GoToDefinition, Redo, Replace, ShowCompletions};

use crate::keys::save_key::CONTEXT;

actions!(
    editorview,
    [
        AddNextOccurrence,
        SelectAllOccurrences,
        MoveLinesUp,
        MoveLinesDown,
        CopyLinesUp,
        CopyLinesDown,
        DeleteLines,
        Fold,
        Unfold,
        FoldAll,
        UnfoldAll,
        GoToLine,
    ]
);

const GO_TO_LINE: &str = "ctrl-g";
const SHOW_COMPLETIONS: &str = "ctrl-space";
const GO_TO_DEFINITION: &str = "f12";

const INPUT_CONTEXT: &str = "Input";

// PLATFORM: macos takes Monaco's mac chords; linux takes its linux chords.
#[cfg(target_os = "macos")]
mod chords {
    pub(super) const ADD_NEXT_OCCURRENCE: &str = "cmd-d";
    pub(super) const SELECT_ALL_OCCURRENCES: &str = "cmd-shift-l";
    pub(super) const COPY_LINES_UP: &str = "shift-alt-up";
    pub(super) const COPY_LINES_DOWN: &str = "shift-alt-down";
    pub(super) const DELETE_LINES: &str = "cmd-shift-k";
    pub(super) const FOLD: &str = "cmd-alt-[";
    pub(super) const UNFOLD: &str = "cmd-alt-]";
    pub(super) const FOLD_ALL: &str = "cmd-k cmd-0";
    pub(super) const UNFOLD_ALL: &str = "cmd-k cmd-j";
    pub(super) const REDO: &str = "cmd-shift-z";
    pub(super) const REPLACE: &str = "alt-cmd-f";
}
#[cfg(not(target_os = "macos"))]
mod chords {
    pub(super) const ADD_NEXT_OCCURRENCE: &str = "ctrl-d";
    pub(super) const SELECT_ALL_OCCURRENCES: &str = "ctrl-shift-l";
    pub(super) const COPY_LINES_UP: &str = "ctrl-shift-alt-up";
    pub(super) const COPY_LINES_DOWN: &str = "ctrl-shift-alt-down";
    pub(super) const DELETE_LINES: &str = "ctrl-shift-k";
    pub(super) const FOLD: &str = "ctrl-shift-[";
    pub(super) const UNFOLD: &str = "ctrl-shift-]";
    pub(super) const FOLD_ALL: &str = "ctrl-k ctrl-0";
    pub(super) const UNFOLD_ALL: &str = "ctrl-k ctrl-j";
    pub(super) const REDO: &str = "ctrl-shift-z";
    pub(super) const REPLACE: &str = "ctrl-h";
}

const MOVE_LINES_UP: &str = "alt-up";
const MOVE_LINES_DOWN: &str = "alt-down";

pub(crate) fn bindings() -> Vec<KeyBinding> {
    let context = Some(CONTEXT);
    vec![
        KeyBinding::new(chords::ADD_NEXT_OCCURRENCE, AddNextOccurrence, context),
        KeyBinding::new(
            chords::SELECT_ALL_OCCURRENCES,
            SelectAllOccurrences,
            context,
        ),
        KeyBinding::new(MOVE_LINES_UP, MoveLinesUp, context),
        KeyBinding::new(MOVE_LINES_DOWN, MoveLinesDown, context),
        KeyBinding::new(chords::COPY_LINES_UP, CopyLinesUp, context),
        KeyBinding::new(chords::COPY_LINES_DOWN, CopyLinesDown, context),
        KeyBinding::new(chords::DELETE_LINES, DeleteLines, context),
        KeyBinding::new(chords::FOLD, Fold, context),
        KeyBinding::new(chords::UNFOLD, Unfold, context),
        KeyBinding::new(chords::FOLD_ALL, FoldAll, context),
        KeyBinding::new(chords::UNFOLD_ALL, UnfoldAll, context),
        KeyBinding::new(chords::REDO, Redo, Some(INPUT_CONTEXT)),
        KeyBinding::new(chords::REPLACE, Replace, context),
        KeyBinding::new(GO_TO_LINE, GoToLine, context),
        KeyBinding::new(SHOW_COMPLETIONS, ShowCompletions, context),
        KeyBinding::new(GO_TO_DEFINITION, GoToDefinition, context),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn no_chord_is_bound_twice_in_one_context() {
        let bound: Vec<String> = bindings()
            .iter()
            .map(|binding| format!("{:?} {:?}", binding.keystrokes(), binding.predicate()))
            .collect();
        let mut unique = bound.clone();
        unique.sort();
        unique.dedup();
        assert_eq!(unique.len(), bound.len(), "{bound:#?}");
    }
}
