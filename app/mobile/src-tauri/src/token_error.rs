use std::fmt;
use std::sync::Arc;

#[derive(Clone, Debug)]
pub(crate) struct TokenError(Arc<keyring::Error>);

impl TokenError {
    pub(crate) fn new(error: keyring::Error) -> Self {
        Self(Arc::new(error))
    }
}

impl fmt::Display for TokenError {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        fmt::Display::fmt(&*self.0, f)
    }
}

impl std::error::Error for TokenError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        Some(&*self.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_copy_reads_like_the_keychain_failure_it_wraps() {
        let error = TokenError::new(keyring::Error::NoStorageAccess("locked".into()));
        let copy = error.clone();
        assert_eq!(copy.to_string(), error.to_string());
        assert!(std::error::Error::source(&copy).is_some());
    }
}
