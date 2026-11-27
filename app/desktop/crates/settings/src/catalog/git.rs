use super::rows::row;
use crate::schema::Control;
use crate::schema::Page;
use crate::schema::Section;
use crate::values::Value;

pub(super) fn page() -> Page {
    Page::new("git", "Git", "git-branch")
        .indexed(super::search_index::entries(super::search_index::GIT))
        .block(Section::new("Git View").headerless().row(row(
            ("compactGitStatusBadges", "Compact Git Status Badges"),
            "Use a denser layout for diff stats and staged labels in the Git panel",
            Control::switch(),
            Value::Bool(false),
        )))
        .block(Section::new("File Tree").row(row(
            ("showGitStatusInFileTree", "Show Git Status In File Tree"),
            "Display Git color decorations in Files",
            Control::switch(),
            Value::Bool(true),
        )))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn two_switches_in_two_sections() {
        let page = page();
        let keys: Vec<&str> = page.rows().map(|row| row.key().as_str()).collect();
        assert_eq!(keys, ["compactGitStatusBadges", "showGitStatusInFileTree"]);
        assert_eq!(page.blocks.len(), 2);
    }
}
