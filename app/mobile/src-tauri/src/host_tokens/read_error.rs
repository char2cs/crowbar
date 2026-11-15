pub(super) struct ReadError {
    pub(super) message: String,
    pub(super) denied: bool,
}

impl ReadError {
    pub(super) fn from_keyring(error: &keyring::Error) -> Self {
        Self {
            message: error.to_string(),
            denied: is_denial(error),
        }
    }
}

fn is_denial(error: &keyring::Error) -> bool {
    !matches!(
        error,
        keyring::Error::NoStorageAccess(_) | keyring::Error::PlatformFailure(_)
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_clearly_transient_failures_are_not_a_denial() {
        let cases = [
            (keyring::Error::NoEntry, true),
            (keyring::Error::PlatformFailure("boom".into()), false),
            (keyring::Error::NoStorageAccess("locked".into()), false),
            (keyring::Error::BadEncoding(vec![0xff]), true),
        ];
        for (error, denied) in cases {
            let read = ReadError::from_keyring(&error);
            assert_eq!(read.denied, denied, "{error}");
            assert_eq!(read.message, error.to_string());
        }
    }
}
