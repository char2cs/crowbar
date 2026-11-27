use std::sync::Arc;

#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct HostId(Arc<str>);

impl From<&str> for HostId {
    fn from(value: &str) -> Self {
        Self(Arc::from(value))
    }
}

impl From<String> for HostId {
    fn from(value: String) -> Self {
        Self(Arc::from(value))
    }
}

impl HostId {
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn identifiers_compare_by_text() {
        let from_str = HostId::from("a");
        let from_string = HostId::from(String::from("a"));
        assert_eq!(from_str, from_string, "same text, same id");
        assert_eq!(from_str.as_str(), "a", "text round trip");
        assert_ne!(from_str, HostId::from("b"), "different text");
    }
}
