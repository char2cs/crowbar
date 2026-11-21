//! How an inline input looks for what it names: a branch name is mono, as develop's
//! `branch-out-input.tsx` draws it.

use sidebar::Editing;
use ui::TextInputStyle;

pub(crate) fn of(editing: &Editing) -> TextInputStyle {
    match editing {
        Editing::Branching(_) | Editing::Drafting(_) => TextInputStyle::BareMono,
        _ => TextInputStyle::Bare,
    }
}

#[cfg(test)]
mod tests {
    use sidebar::RowId;

    use super::*;

    #[test]
    fn a_branch_name_is_mono_and_a_rename_is_not() {
        let row = RowId::from("chat");
        assert_eq!(
            of(&Editing::Branching(row.clone())),
            TextInputStyle::BareMono
        );
        assert_eq!(
            of(&Editing::Drafting(row.clone())),
            TextInputStyle::BareMono
        );
        assert_eq!(of(&Editing::Renaming(row)), TextInputStyle::Bare);
    }
}
