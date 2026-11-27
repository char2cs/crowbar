pub trait FileAccess {
    fn files_locked(&self) -> bool {
        false
    }
    fn files_are_local(&self) -> bool {
        true
    }
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
