//! How an inline input looks for what it names: a branch name is mono, as develop's
//! `branch-out-input.tsx` draws it, and a file name is framed, as `file-explorer-tree-item.tsx`.

use sidebar::Editing;
use ui::TextInputStyle;

pub(crate) fn of(editing: &Editing) -> TextInputStyle {
    match editing {
        Editing::Branching(_) | Editing::Drafting(_) => TextInputStyle::BareMono,
        Editing::RenamingFile(_) | Editing::Creating { .. } => TextInputStyle::Outlined,
        _ => TextInputStyle::Bare,
    }
}

#[cfg(test)]
mod tests {
    use sidebar::RowId;

    use super::*;

    #[test]
    fn a_branch_name_is_mono_a_file_name_framed_and_a_rename_bare() {
        let row = RowId::from("chat");
        let cases = [
            (Editing::Branching(row.clone()), TextInputStyle::BareMono),
            (Editing::Drafting(row.clone()), TextInputStyle::BareMono),
            (Editing::Renaming(row), TextInputStyle::Bare),
            (
                Editing::RenamingFile("README.md".into()),
                TextInputStyle::Outlined,
            ),
        ];
        for (editing, style) in cases {
            assert_eq!(of(&editing), style, "{editing:?}");
        }
    }
}
