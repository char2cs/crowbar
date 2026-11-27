use std::sync::Arc;

#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct ProjectId(Arc<str>);

impl From<&str> for ProjectId {
    fn from(value: &str) -> Self {
        Self(Arc::from(value))
    }
}

impl From<String> for ProjectId {
    fn from(value: String) -> Self {
        Self(Arc::from(value))
    }
}

impl ProjectId {
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn identifiers_compare_by_text() {
        let from_str = ProjectId::from("a");
        let from_string = ProjectId::from(String::from("a"));
        assert_eq!(from_str, from_string, "same text, same id");
        assert_eq!(from_str.as_str(), "a", "text round trip");
        assert_ne!(from_str, ProjectId::from("b"), "different text");
    }
}
