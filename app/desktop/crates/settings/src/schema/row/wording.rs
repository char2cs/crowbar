use crate::schema::condition::Condition;
use crate::schema::row::Row;

impl Row {
    #[must_use]
    pub(crate) fn describe(
        mut self,
        text: impl Into<String>,
    ) -> Self {
        self.description = Some(text.into());
        self
    }

    #[must_use]
    pub(crate) fn describe_invalid(
        mut self,
        text: impl Into<String>,
    ) -> Self {
        self.invalid_description = Some(text.into());
        self
    }

    #[must_use]
    pub(crate) fn describe_while(
        mut self,
        (condition, text): (Condition, &str),
    ) -> Self {
        self.worded = Some((condition, text.to_owned()));
        self
    }
}

#[cfg(test)]
mod tests {
    use crate::schema::condition::Condition;
    use crate::schema::control::Control;
    use crate::schema::row::Row;

    #[test]
    fn each_text_under_the_label_is_kept_for_its_case() {
        let row = Row::new("tabSize", "Tab Size", Control::switch())
            .describe("Spaces per tab")
            .describe_invalid("Bad")
            .describe_while((Condition::capability("tabs"), "Fixed"));
        assert_eq!(row.description.as_deref(), Some("Spaces per tab"));
        assert_eq!(row.invalid_description.as_deref(), Some("Bad"));
        let worded = row.worded.as_ref().map(|(_, text)| text.as_str());
        assert_eq!(worded, Some("Fixed"));
    }
}
