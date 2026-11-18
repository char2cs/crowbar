//! How many connections hold a chat, as the projects scope carries it.

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub(crate) struct PresenceRecord {
    pub(crate) viewers: u32,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_presence_reads_its_viewers_and_defaults_to_none() {
        let seen: PresenceRecord = serde_json::from_str(r#"{"viewers":2}"#).unwrap();
        assert_eq!(seen.viewers, 2);
        assert_eq!(PresenceRecord::default().viewers, 0);
    }
}
