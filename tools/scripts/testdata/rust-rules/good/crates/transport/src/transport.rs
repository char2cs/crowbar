use crate::{Error, HostToken};

/// Opens a connection with the given token.
///
/// # Errors
///
/// Returns [`Error::Refused`] when the token is `refused`.
pub fn connect(token: &HostToken) -> Result<(), Error> {
    // SAFETY: not unsafe; fixture for the allowed comment tag.
    if token.as_str() == "refused" {
        return Err(Error::Refused);
    }
    Ok(())
}

#[cfg(test)]
#[path = "transport_tests.rs"]
mod tests;
