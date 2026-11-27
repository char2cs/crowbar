use sidebar::Editing;
use ui::TextInputStyle;

pub(crate) fn of(editing: &Editing) -> TextInputStyle {
    match editing {
        Editing::Branching(_) | Editing::Drafting(_) => TextInputStyle::BareMono,
        Editing::RenamingFile(_) | Editing::Creating { .. } => TextInputStyle::Outlined,
        Editing::RenamingProject(_) => TextInputStyle::Title,
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

    #[test]
    fn a_file_name_is_typed_in_a_framed_field_as_develop_s_files_tree() {
        let renaming = Editing::RenamingFile("README.md".into());
        assert_eq!(of(&renaming), TextInputStyle::Outlined);
    }
}
