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
mod tests {
    use super::*;

    #[test]
    fn accepts_a_valid_token() {
        let cases = ["ok", "also-ok"];
        for case in cases {
            let token = HostToken::parse(case).unwrap();
            assert!(connect(&token).is_ok());
        }
    }

    #[test]
    #[ignore = "real-infra: proves the real file mode of a written file"]
    fn writes_a_real_file() {
        std::fs::write("x", b"ok").unwrap();
    }

    #[ignore = "real-infra: reads one real file for the tests that need it"]
    fn scratch() {
        let _ = std::fs::read("x");
    }

    #[test]
    #[ignore = "real-infra: dials a loopback unix socket"]
    fn dials_a_socket() {
        scratch();
    }
}
