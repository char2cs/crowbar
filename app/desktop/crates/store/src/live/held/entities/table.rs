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
        let stale = self.rows.get(id).is_some_and(|(held, _)| version < *held);
        if !stale {
            self.rows.insert(id.to_owned(), (version, row));
        }
    }

    pub(crate) fn patch(
        &mut self,
        id: &str,
        change: impl FnOnce(&mut T),
    ) {
        if let Some((_, row)) = self.rows.get_mut(id) {
            change(row);
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
        Patch(&'static str, &'static str),
        Void,
        Remove(&'static str),
        Forget(&'static str),
    }

    use Step::{Forget, Patch, Put, Remove, Void};

    fn values(steps: &[Step]) -> Vec<&'static str> {
        let mut table = Table::default();
        for step in steps {
            match step {
                Put(key, version, value) => table.put(key, *version, *value),
                Patch(key, value) => table.patch(key, |row| *row = *value),
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

    fn check(cases: Vec<Scenario>) {
        for (name, steps, expected) in cases {
            assert_eq!(values(&steps), expected, "{name}");
        }
    }

    #[test]
    fn versions_and_patches_decide_which_write_a_row_keeps() {
        check(vec![
            (
                "an older version is ignored",
                vec![Put("a", 5, "five"), Put("a", 4, "four")],
                vec!["five"],
            ),
            (
                "the same version replaces",
                vec![Put("a", 5, "five"), Put("a", 5, "again")],
                vec!["again"],
            ),
            (
                "a patch edits the held row",
                vec![
                    Put("a", 5, "five"),
                    Patch("a", "placed"),
                    Patch("b", "none"),
                ],
                vec!["placed"],
            ),
            (
                "the frame after a patch wins at the version the patch kept",
                vec![
                    Put("a", 5, "five"),
                    Patch("a", "placed"),
                    Put("a", 5, "frame"),
                ],
                vec!["frame"],
            ),
            (
                "a newer version replaces",
                vec![Put("a", 5, "five"), Put("a", 6, "six")],
                vec!["six"],
            ),
        ]);
    }

    #[test]
    fn epochs_removal_and_retention_decide_what_a_table_holds() {
        check(vec![
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
        ]);
    }
}
