use std::fmt;

#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct PageId(String);

impl PageId {
    pub fn new(id: impl Into<String>) -> Self {
        Self(id.into())
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl From<&str> for PageId {
    fn from(id: &str) -> Self {
        Self::new(id)
    }
}

impl fmt::Display for PageId {
    fn fmt(
        &self,
        formatter: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trips_its_text() {
        let id = PageId::from("keybindings");
        assert_eq!(
            (id.as_str(), id.to_string()),
            ("keybindings", "keybindings".to_owned())
        );
    }
}
