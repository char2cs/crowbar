//! Reading the document: stored values, defaults, drafts and whether a row is dirty.

use crate::schema::Row;
use crate::state::document::Document;
use crate::values::Key;
use crate::values::Value;
use crate::values::Values;

impl Document {
    pub(crate) fn value(
        &self,
        key: &Key,
    ) -> Option<&Value> {
        self.values.get(key).or_else(|| self.defaults.get(key))
    }

    pub(crate) fn values(&self) -> &Values {
        &self.values
    }

    pub(crate) fn defaults(&self) -> &Values {
        &self.defaults
    }

    pub(crate) fn is_default(
        &self,
        row: &Row,
    ) -> bool {
        row.default
            .as_ref()
            .is_none_or(|default| self.value(&row.key) == Some(default))
    }

    pub(crate) fn text(
        &self,
        row: &Row,
    ) -> String {
        match self.drafts.get(&row.key) {
            Some(draft) => draft.clone(),
            None => row.control.text_of(self.value(&row.key)),
        }
    }

    pub(crate) fn draft(
        &self,
        key: &Key,
    ) -> Option<&str> {
        self.drafts.get(key).map(String::as_str)
    }

    pub(crate) fn is_dirty(
        &self,
        row: &Row,
    ) -> bool {
        self.drafts
            .get(&row.key)
            .is_some_and(|draft| *draft != row.control.text_of(self.value(&row.key)))
    }

    pub(crate) fn has_unsaved(&self) -> bool {
        !self.unsaved.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::schema::Control;
    use crate::schema::ControlWidth;
    use crate::schema::NumberRange;

    fn size_row() -> Row {
        Row::new(
            "tabSize",
            "Tab Size",
            Control::number(NumberRange::new(1.0, 8.0, 1.0), ControlWidth::NumberCompact),
        )
        .default_value(2.0)
    }

    fn document() -> Document {
        Document::new(Values::default(), Values::default().with("tabSize", 2.0))
    }

    #[test]
    fn defaults_show_until_a_value_is_stored() {
        let mut document = document();
        let row = size_row();
        assert_eq!(document.value(&row.key), Some(&Value::Number(2.0)));
        assert!(document.values().get(&row.key).is_none());
        assert!(document.is_default(&row));
        document.apply(&row.key, Value::Number(4.0));
        assert!(!document.is_default(&row));
        assert_eq!(document.text(&row), "4");
    }

    #[test]
    fn drafts_track_dirtiness_until_reverted_or_applied() {
        let mut document = document();
        let row = size_row();
        document.edit(&row.key, "5");
        assert!(document.is_dirty(&row));
        assert_eq!(document.text(&row), "5");
        assert_eq!(document.draft(&row.key), Some("5"));
        document.edit(&row.key, "2");
        assert!(
            !document.is_dirty(&row),
            "typing the stored value back is not dirty"
        );
        document.revert(&row.key);
        assert_eq!(document.draft(&row.key), None);
        document.edit(&row.key, "7");
        document.apply(&row.key, Value::Number(7.0));
        assert_eq!(document.draft(&row.key), None, "applying clears the draft");
        assert_eq!(document.values().get(&row.key), Some(&Value::Number(7.0)));
        assert_eq!(document.defaults().get(&row.key), Some(&Value::Number(2.0)));
    }
}
