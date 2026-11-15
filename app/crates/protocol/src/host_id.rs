use crate::error::Result;
use crate::string_id::{check_text, string_id};

fn validate(value: &str) -> Result<()> {
    check_text("host id", value, |c| {
        c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-'
    })
}

string_id!(
    /// Identifies a daemon host: `local` or a paired remote host. It is the authority of
    /// `crowbar://<host>/...` URLs, so it holds only lower case letters, digits and hyphens.
    HostId,
    validate
);

#[cfg(test)]
mod tests {
    use super::*;
    use crate::error::Error;

    #[test]
    fn accepts_local_and_paired_hosts() {
        assert!(HostId::new("local").is_ok(), "local");
        assert!(HostId::new("host-7b").is_ok(), "paired");
    }

    #[test]
    fn rejects_authority_breaking_text() {
        assert!(
            matches!(
                HostId::new("a/b"),
                Err(Error::InvalidIdCharacter { found: '/', .. })
            ),
            "slash"
        );
        assert!(HostId::new("Upper").is_err(), "upper case");
        assert!(
            HostId::new("host_7").is_err(),
            "underscore is not a hostname character"
        );
        assert!(HostId::new("").is_err(), "empty");
    }

    #[test]
    fn display_parse_and_serde_round_trip() {
        let id: HostId = "local".parse().unwrap();
        assert_eq!(id.to_string(), "local", "display");
        assert_eq!(id.as_ref(), "local", "as_ref");
        assert_eq!(
            serde_json::to_string(&id).unwrap(),
            "\"local\"",
            "serialize"
        );
        assert!(
            serde_json::from_str::<HostId>("\"A\"").is_err(),
            "deserialize validates"
        );
        assert_eq!(String::from(id), "local", "into string");
    }
}
