use std::sync::Arc;

#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct RowId(Arc<str>);

impl From<&str> for RowId {
    fn from(value: &str) -> Self {
        Self(Arc::from(value))
    }
}

impl From<String> for RowId {
    fn from(value: String) -> Self {
        Self(Arc::from(value))
    }
}

impl RowId {
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn identifiers_compare_by_text() {
        let from_str = RowId::from("a");
        let from_string = RowId::from(String::from("a"));
        assert_eq!(from_str, from_string, "same text, same id");
        assert_eq!(from_str.as_str(), "a", "text round trip");
        assert_ne!(from_str, RowId::from("b"), "different text");
    }
}
