//! A list the daemon is asked for: still loading, failed, or read.

use settings_ui::{Record, Records};

#[derive(Debug, Clone)]
pub(crate) enum Loadable<T> {
    Loading,
    Failed,
    Ready(T),
}

impl<T> Loadable<T> {
    pub(crate) fn records(
        &self,
        of: impl FnOnce(&T) -> Vec<Record>,
    ) -> Records {
        match self {
            Self::Loading => Records::Loading,
            Self::Failed => Records::Failed,
            Self::Ready(held) => Records::Ready(of(held)),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_state_reads_as_its_records_state() {
        let ready = Loadable::Ready(2).records(|count| vec![Record::new("a", "a"); *count]);
        assert!(matches!(ready, Records::Ready(rows) if rows.len() == 2));
        assert!(matches!(
            Loadable::<u8>::Loading.records(|_| Vec::new()),
            Records::Loading
        ));
        assert!(matches!(
            Loadable::<u8>::Failed.records(|_| Vec::new()),
            Records::Failed
        ));
    }
}
