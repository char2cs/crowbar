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

    const CASES: [(&str, bool); 6] = [
        ("local", true),
        ("host-7b", true),
        ("a/b", false),
        ("Upper", false),
        ("host_7", false),
        ("", false),
    ];

    #[test]
    fn validity_follows_the_hostname_alphabet() {
        for (text, valid) in CASES {
            assert_eq!(HostId::new(text).is_ok(), valid, "{text:?}");
        }
        assert!(
            matches!(
                HostId::new("a/b"),
                Err(Error::InvalidIdCharacter { found: '/', .. })
            ),
            "slash names the offending character"
        );
    }

    #[test]
    fn display_parse_and_serde_round_trip() {
        let id: HostId = "local".parse().unwrap();
        assert_eq!(id.to_string(), "local", "display");
        assert_eq!(
            serde_json::to_string(&id).unwrap(),
            "\"local\"",
            "serialize"
        );
        assert!(
            serde_json::from_str::<HostId>("\"A\"").is_err(),
            "deserialize validates"
        );
    }
}
