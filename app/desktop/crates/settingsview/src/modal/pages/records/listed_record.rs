use settings::Record;

#[derive(Debug, Clone, Copy)]
pub(crate) struct ListedRecord<'a> {
    pub(crate) source: &'a str,
    pub(crate) record: &'a Record,
    pub(crate) index: usize,
}

impl<'a> ListedRecord<'a> {
    pub(crate) fn new(
        source: &'a str,
        record: &'a Record,
        index: usize,
    ) -> Self {
        Self {
            source,
            record,
            index,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn keeps_the_list_the_record_and_the_position() {
        let record = Record::new("a", "Title");
        let reference = ListedRecord::new("projects", &record, 3);
        assert_eq!(reference.source, "projects");
        assert_eq!(reference.record.id(), "a");
        assert_eq!(reference.index, 3);
    }
}
