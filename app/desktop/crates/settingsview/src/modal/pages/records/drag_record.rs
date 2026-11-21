//! A record lifted by its grip to be dropped at another place of its list.

use gpui::{App, Window};

use crate::intent::Intent;
use crate::intent::Sink;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DragRecord {
    pub(crate) source: String,
    pub(crate) record: String,
}

impl DragRecord {
    pub(crate) fn dropped_at(
        &self,
        (source, index): (&str, usize),
    ) -> Option<Intent> {
        (self.source == source).then(|| Intent::Reorder {
            source: self.source.clone(),
            record: self.record.clone(),
            to: index,
        })
    }
}

impl DragRecord {
    pub(crate) fn deliver(
        &self,
        sink: &Sink,
        target: (&str, usize),
        (window, cx): (&mut Window, &mut App),
    ) {
        let Some(intent) = self.dropped_at(target) else {
            return;
        };
        sink.deliver(intent, (window, cx));
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_record_moves_only_within_its_own_list() {
        let drag = DragRecord {
            source: "agents".into(),
            record: "codex".into(),
        };
        let cases = [
            (
                ("agents", 0),
                Some(r#"Reorder { source: "agents", record: "codex", to: 0 }"#),
            ),
            (("sessions", 0), None),
        ];
        for (target, expected) in cases {
            let moved = drag.dropped_at(target).map(|intent| format!("{intent:?}"));
            assert_eq!(moved.as_deref(), expected, "{target:?}");
        }
    }
}
