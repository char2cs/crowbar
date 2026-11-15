//! How the daemon's git file statuses read in the trees.

use sidebar::GitStatus;

pub(crate) fn status_of(word: &str) -> GitStatus {
    match word {
        "added" => GitStatus::Added,
        "deleted" => GitStatus::Deleted,
        "untracked" => GitStatus::Untracked,
        _ => GitStatus::Modified,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn renames_and_conflicts_read_as_modified() {
        assert_eq!(status_of("added"), GitStatus::Added);
        assert_eq!(status_of("deleted"), GitStatus::Deleted);
        assert_eq!(status_of("renamed"), GitStatus::Modified);
    }
}
