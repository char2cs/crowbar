/// A bearer token for one host.
#[derive(Debug)]
pub struct HostToken {
    value: String,
}

impl HostToken {
    /// Wraps a non-empty token, or returns `None`.
    pub fn parse(value: &str) -> Option<Self> {
        let is_blank = value.trim().is_empty();
        if is_blank {
            return None;
        }
        Some(Self { value: value.to_owned() })
    }

    /// The raw token text.
    pub fn as_str(&self) -> &str {
        &self.value
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_blank_tokens() {
        assert!(HostToken::parse("  ").is_none());
    }
}
