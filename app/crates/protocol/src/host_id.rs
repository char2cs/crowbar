use crate::error::Result;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("host id", value, |c| {
        c.is_ascii_lowercase() || c.is_ascii_digit() || matches!(c, '-' | '_')
    })
}

string_id!(
    /// Identifies a daemon host: `local` or a paired remote host. It is the authority of
    /// `crowbar://<host>/...` URLs.
    HostId,
    validate
);

#[cfg(test)]
#[path = "host_id_tests.rs"]
mod tests;
