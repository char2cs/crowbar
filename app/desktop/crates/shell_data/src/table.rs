//! The rows of one entity kind, each with the version that wrote it.

use std::collections::BTreeMap;

#[derive(Debug, Clone)]
pub(crate) struct Table<T> {
    rows: BTreeMap<String, (u64, T)>,
}

impl<T> Default for Table<T> {
    fn default() -> Self {
        Self {
            rows: BTreeMap::new(),
        }
    }
}

impl<T> Table<T> {
    pub(crate) fn put(
        &mut self,
        id: &str,
        version: u64,
        row: T,
    ) {
        let stale = self.rows.get(id).is_some_and(|(held, _)| version <= *held);
        if !stale {
            self.rows.insert(id.to_owned(), (version, row));
        }
    }

    pub(crate) fn remove(
        &mut self,
        id: &str,
    ) {
        self.rows.remove(id);
    }

    pub(crate) fn retain(
        &mut self,
        mut keep: impl FnMut(&str, &T) -> bool,
    ) {
        self.rows.retain(|id, (_, row)| keep(id, row));
    }

    pub(crate) fn void_versions(&mut self) {
        self.rows.values_mut().for_each(|(version, _)| *version = 0);
    }

    pub(crate) fn get(
        &self,
        id: &str,
    ) -> Option<&T> {
        self.rows.get(id).map(|(_, row)| row)
    }

    pub(crate) fn values(&self) -> impl Iterator<Item = &T> {
        self.rows.values().map(|(_, row)| row)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    enum Step {
        Put(&'static str, u64, &'static str),
        Void,
        Remove(&'static str),
        Forget(&'static str),
    }

    use Step::{Forget, Put, Remove, Void};

    fn values(steps: &[Step]) -> Vec<&'static str> {
        let mut table = Table::default();
        for step in steps {
            match step {
                Put(key, version, value) => table.put(key, *version, *value),
                Void => table.void_versions(),
                Remove(key) => table.remove(key),
                Forget(value) => table.retain(|_, row| row != value),
            }
        }
        let mut held: Vec<&'static str> = table.values().copied().collect();
        held.sort_unstable();
        held
    }

    type Scenario = (&'static str, Vec<Step>, Vec<&'static str>);

    #[test]
    fn versions_epochs_removal_and_retention_decide_what_a_table_holds() {
        let cases: [Scenario; 5] = [
            (
                "an older version is ignored",
                vec![
                    Put("a", 5, "five"),
                    Put("a", 4, "four"),
                    Put("a", 5, "again"),
                ],
                vec!["five"],
            ),
            (
                "a newer version replaces",
                vec![Put("a", 5, "five"), Put("a", 6, "six")],
                vec!["six"],
            ),
            (
                "a new epoch lets any version replace",
                vec![Put("a", 9, "old"), Void, Put("a", 1, "new")],
                vec!["new"],
            ),
            (
                "remove forgets a row",
                vec![Put("a", 1, "one"), Put("b", 1, "two"), Remove("a")],
                vec!["two"],
            ),
            (
                "retain forgets rows by value",
                vec![Put("a", 1, "one"), Put("b", 1, "two"), Forget("two")],
                vec!["one"],
            ),
        ];
        for (name, steps, expected) in cases {
            assert_eq!(values(&steps), expected, "{name}");
        }
    }
}
