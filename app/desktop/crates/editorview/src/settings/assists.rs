use crate::settings::highlighting::Highlighting;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Assists {
    pub completion: bool,
    pub occurrences: bool,
    pub highlighting: Highlighting,
}

impl Default for Assists {
    fn default() -> Self {
        Self {
            completion: true,
            occurrences: false,
            highlighting: Highlighting::Semantic,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nothing_stored_helps_as_develop_does() {
        let develop = Assists {
            completion: true,
            occurrences: false,
            highlighting: Highlighting::Semantic,
        };
        assert_eq!(Assists::default(), develop);
    }
}
