//! One review thread of the daemon's list, as far as the merge box needs it.

use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
pub(crate) struct ThreadBody {
    pub(crate) resolved: bool,
}

pub(crate) fn open_in(threads: &[ThreadBody]) -> u32 {
    let open = threads.iter().filter(|thread| !thread.resolved).count();
    u32::try_from(open).unwrap_or(u32::MAX)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_unresolved_threads_are_open() {
        let threads: Vec<ThreadBody> =
            serde_json::from_str(r#"[{"resolved":true},{"resolved":false},{}]"#).unwrap();
        assert_eq!(open_in(&threads), 2);
    }
}
