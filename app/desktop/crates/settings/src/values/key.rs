use std::fmt;

#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct Key(String);

impl Key {
    pub fn new(name: impl Into<String>) -> Self {
        Self(name.into())
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl From<&str> for Key {
    fn from(name: &str) -> Self {
        Self::new(name)
    }
}

impl fmt::Display for Key {
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
    fn displays_as_its_name() {
        assert_eq!(Key::from("tabSize").to_string(), "tabSize");
        assert_eq!(Key::from("tabSize").as_str(), "tabSize");
    }
}
