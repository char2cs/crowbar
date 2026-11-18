use crate::records::record::Record;
use crate::records::record_action::RecordAction;
use crate::records::record_flag::RecordFlag;

impl Record {
    /// Appends a switch.
    #[must_use]
    pub fn flag(
        mut self,
        flag: RecordFlag,
    ) -> Self {
        self.flags.push(flag);
        self
    }

    /// Offers `action` on the record.
    #[must_use]
    pub fn action(
        mut self,
        action: RecordAction,
    ) -> Self {
        self.action = Some(action);
        self
    }

    /// Lets the user drag the record to reorder the list.
    #[must_use]
    pub fn movable(mut self) -> Self {
        self.movable = true;
        self
    }

    /// Switches, each shown in the column of the same position.
    #[must_use]
    pub fn flags(&self) -> &[RecordFlag] {
        &self.flags
    }

    /// The one action the record offers, if any.
    #[must_use]
    pub fn offered_action(&self) -> Option<&RecordAction> {
        self.action.as_ref()
    }

    /// Whether the user can drag the record to reorder the list.
    #[must_use]
    pub fn is_movable(&self) -> bool {
        self.movable
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn switches_keep_their_columns_and_dragging_is_opt_in() {
        let bare = Record::new("claude", "Claude Code");
        assert!(bare.offered_action().is_none() && !bare.is_movable());
        let record = bare
            .flag(RecordFlag::new("enabled", "Enable").on())
            .flag(RecordFlag::new("default", "Default"))
            .action(RecordAction::new("kill", "skull", "Kill"))
            .movable();
        let columns: Vec<(&str, bool)> = record
            .flags()
            .iter()
            .map(|flag| (flag.id(), flag.is_on()))
            .collect();
        assert_eq!(columns, [("enabled", true), ("default", false)]);
        assert!(record.offered_action().is_some() && record.is_movable());
    }
}
