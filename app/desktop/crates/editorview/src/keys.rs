use gpui::{KeyBinding, actions};
use gpui_kit::base::input::{GoToDefinition, Redo, Replace, Search, ShowCompletions};

use crate::save_key::CONTEXT;

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
        FindNext,
        FindPrevious,
        ToggleMatchCase,
        ToggleWholeWord,
        ToggleRegex,
        ToggleFindInSelection,
        ReplaceOne,
        ReplaceAll,
        CloseFind,
        GoToLine,
    ]
);

pub(crate) const FIND_CONTEXT: &str = "CrowbarFind";
const FIND_FIELD: &str = "CrowbarFind > UiTextInput";
const FIND_NEXT: &str = "f3";
const FIND_PREVIOUS: &str = "shift-f3";
const PREVIOUS_IN_FIELD: &str = "shift-enter";
const CLOSE_FIND: &str = "escape";
// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/standalone/browser/quickAccess/standaloneGotoLineQuickAccess.ts
const GO_TO_LINE: &str = "ctrl-g";
// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/suggest/browser/suggestController.ts
const SHOW_COMPLETIONS: &str = "ctrl-space";
// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/gotoSymbol/browser/goToCommands.ts
const GO_TO_DEFINITION: &str = "f12";

const INPUT_CONTEXT: &str = "Input";

// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/linesOperations/browser/linesOperations.ts
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
    pub(super) const FIND: &str = "cmd-f";
    pub(super) const REPLACE: &str = "alt-cmd-f";
    pub(super) const MATCH_CASE: &str = "alt-cmd-c";
    pub(super) const WHOLE_WORD: &str = "alt-cmd-w";
    pub(super) const REGEX: &str = "alt-cmd-r";
    pub(super) const IN_SELECTION: &str = "alt-cmd-l";
    pub(super) const REPLACE_ONE: &str = "cmd-shift-1";
    pub(super) const REPLACE_ALL: &str = "cmd-alt-enter";
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
    pub(super) const FIND: &str = "ctrl-f";
    pub(super) const REPLACE: &str = "ctrl-h";
    pub(super) const MATCH_CASE: &str = "alt-c";
    pub(super) const WHOLE_WORD: &str = "alt-w";
    pub(super) const REGEX: &str = "alt-r";
    pub(super) const IN_SELECTION: &str = "alt-l";
    pub(super) const REPLACE_ONE: &str = "ctrl-shift-1";
    pub(super) const REPLACE_ALL: &str = "ctrl-alt-enter";
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
        KeyBinding::new(chords::FIND, Search, context),
        KeyBinding::new(chords::REPLACE, Replace, context),
        KeyBinding::new(FIND_NEXT, FindNext, context),
        KeyBinding::new(FIND_PREVIOUS, FindPrevious, context),
        KeyBinding::new(PREVIOUS_IN_FIELD, FindPrevious, Some(FIND_FIELD)),
        KeyBinding::new(chords::MATCH_CASE, ToggleMatchCase, context),
        KeyBinding::new(chords::WHOLE_WORD, ToggleWholeWord, context),
        KeyBinding::new(chords::REGEX, ToggleRegex, context),
        KeyBinding::new(chords::IN_SELECTION, ToggleFindInSelection, context),
        KeyBinding::new(chords::REPLACE_ONE, ReplaceOne, context),
        KeyBinding::new(chords::REPLACE_ALL, ReplaceAll, context),
        KeyBinding::new(CLOSE_FIND, CloseFind, context),
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
