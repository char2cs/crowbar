const CONFIRM_ABOVE: usize = 100;

#[derive(Debug, PartialEq, Eq)]
pub(crate) enum OpenAll {
    Nothing,
    Now(Vec<String>),
    Ask(Vec<String>),
}

pub(crate) fn plan(mut files: Vec<String>) -> OpenAll {
    files.dedup();
    if files.is_empty() {
        return OpenAll::Nothing;
    }
    if files.len() > CONFIRM_ABOVE {
        return OpenAll::Ask(files);
    }
    OpenAll::Now(files)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn files(count: usize) -> Vec<String> {
        (0..count).map(|index| format!("f{index:03}")).collect()
    }

    #[test]
    fn only_a_large_directory_is_asked_about() {
        assert_eq!(plan(Vec::new()), OpenAll::Nothing);
        assert_eq!(plan(files(1)), OpenAll::Now(files(1)));
        assert_eq!(plan(files(100)), OpenAll::Now(files(100)));
        assert_eq!(plan(files(101)), OpenAll::Ask(files(101)));
    }

    #[test]
    fn a_repeated_neighbour_is_opened_once() {
        let twice = vec!["a".to_owned(), "a".to_owned()];
        assert_eq!(plan(twice), OpenAll::Now(vec!["a".to_owned()]));
    }
}
