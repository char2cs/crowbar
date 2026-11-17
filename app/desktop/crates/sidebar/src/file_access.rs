//! What the focused workspace allows on its files.

/// What the focused workspace allows on its files.
pub trait FileAccess {
    /// Whether the focused workspace refuses writes, which hides the entries that change files.
    fn files_locked(&self) -> bool {
        false
    }
    /// Whether the workspace is on this machine, which shows the file manager entry.
    fn files_are_local(&self) -> bool {
        true
    }
    /// Whether the shell can generate images, which shows that entry on directories.
    fn can_generate_image(&self) -> bool {
        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Plain;

    impl FileAccess for Plain {}

    #[test]
    fn a_source_that_says_nothing_is_open_local_and_cannot_generate_images() {
        assert!(!Plain.files_locked());
        assert!(Plain.files_are_local());
        assert!(!Plain.can_generate_image());
    }
}
