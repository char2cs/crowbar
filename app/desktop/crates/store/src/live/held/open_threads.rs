use std::collections::BTreeMap;

const GENERAL: &str = "";

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct OpenThreads {
    by_path: BTreeMap<String, u32>,
}

impl OpenThreads {
    pub(crate) fn count(
        &mut self,
        path: &str,
    ) {
        *self.by_path.entry(path.to_owned()).or_default() += 1;
    }

    pub(crate) fn total(&self) -> u32 {
        self.by_path.values().sum()
    }

    pub(crate) fn by_file(&self) -> BTreeMap<String, u32> {
        let anchored = self
            .by_path
            .iter()
            .filter(|(path, _)| path.as_str() != GENERAL);
        anchored.map(|(path, open)| (path.clone(), *open)).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_general_thread_counts_in_the_total_but_belongs_to_no_file() {
        let mut open = OpenThreads::default();
        open.count("a.rs");
        open.count(GENERAL);
        assert_eq!(open.total(), 2);
        assert_eq!(open.by_file(), BTreeMap::from([("a.rs".to_owned(), 1)]));
    }
}
