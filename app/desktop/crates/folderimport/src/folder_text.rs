//! Reading a typed folder path as develop's dialogs do.

pub(crate) const NOT_ABSOLUTE: &str = "Enter an absolute path (e.g. /Users/you/code/my-repo)";

pub(crate) fn absolute(path: &str) -> bool {
    path.starts_with('/')
}

pub(crate) fn base_name(path: &str) -> &str {
    let trimmed = path.trim_end_matches('/');
    trimmed.rsplit('/').next().unwrap_or(trimmed)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_rooted_path_is_absolute_and_its_base_name_ignores_trailing_slashes() {
        assert!(absolute("/srv/app"));
        assert!(!absolute("code/x"));
        assert!(!absolute(""));
        assert_eq!(base_name("/srv/app/"), "app");
        assert_eq!(base_name("/srv/app"), "app");
        assert_eq!(base_name("app"), "app");
    }
}
