use crate::error::{Error, Result};

const MAX_ID_BYTES: usize = 128;

pub(crate) fn check_text(
    kind: &'static str,
    value: &str,
    allowed: impl Fn(char) -> bool,
) -> Result<()> {
    if value.is_empty() {
        return Err(Error::EmptyId { kind });
    }
    if value.len() > MAX_ID_BYTES {
        return Err(Error::IdTooLong {
            kind,
            max: MAX_ID_BYTES,
        });
    }
    match value.chars().find(|c| !allowed(*c)) {
        Some(found) => Err(Error::InvalidIdCharacter { kind, found }),
        None => Ok(()),
    }
}

pub(crate) fn is_word_char(c: char) -> bool {
    c.is_ascii_alphanumeric() || matches!(c, '-' | '_' | ':' | '.')
}

macro_rules! string_id {
    ($(#[$meta:meta])* $name:ident, $validate:path) => {
        $(#[$meta])*
        #[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord, serde::Serialize, serde::Deserialize)]
        #[serde(try_from = "String", into = "String")]
        pub struct $name(String);

        impl $name {
            /// Validates and wraps `value`.
            ///
            /// # Errors
            /// Returns an error when `value` is not a valid identifier of this kind.
            pub fn new(value: impl Into<String>) -> $crate::Result<Self> {
                let value = value.into();
                $validate(&value)?;
                Ok(Self(value))
            }

            /// The identifier as a string slice.
            #[must_use]
            pub fn as_str(&self) -> &str {
                &self.0
            }
        }

        impl TryFrom<String> for $name {
            type Error = $crate::Error;
            fn try_from(value: String) -> $crate::Result<Self> {
                Self::new(value)
            }
        }

        impl From<$name> for String {
            fn from(id: $name) -> String {
                id.0
            }
        }

        impl std::str::FromStr for $name {
            type Err = $crate::Error;
            fn from_str(value: &str) -> $crate::Result<Self> {
                Self::new(value)
            }
        }

        impl std::fmt::Display for $name {
            fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                f.write_str(&self.0)
            }
        }

        impl AsRef<str> for $name {
            fn as_ref(&self) -> &str {
                &self.0
            }
        }
    };
}

pub(crate) use string_id;

#[cfg(test)]
#[path = "string_id_tests.rs"]
mod tests;
