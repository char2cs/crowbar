//! The record a drag holds up and the slot it hovers, so the list shows the order it would drop into.
//! Spec: web/src/features/settings/components/tabs/providers-settings-view.tsx (dnd-kit sortable).

use settings::Record;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Lift {
    pub(crate) source: String,
    pub(crate) record: String,
    pub(crate) over: usize,
}

impl Lift {
    pub(crate) fn holds(
        &self,
        source: &str,
        record: &Record,
    ) -> bool {
        self.source == source && self.record == record.id()
    }
}

pub(crate) fn arranged<'a>(
    lift: Option<&Lift>,
    source: &str,
    records: &'a [Record],
) -> Vec<&'a Record> {
    let mut order: Vec<&Record> = records.iter().collect();
    let Some(lift) = lift.filter(|lift| lift.source == source) else {
        return order;
    };
    let Some(from) = order.iter().position(|record| record.id() == lift.record) else {
        return order;
    };
    let held = order.remove(from);
    order.insert(lift.over.min(order.len()), held);
    order
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (&'static str, usize, [&'static str; 3]);

    fn lifted(
        source: &str,
        over: usize,
    ) -> Lift {
        Lift {
            source: source.to_owned(),
            record: "a".to_owned(),
            over,
        }
    }

    fn ids<'a>(order: &[&'a Record]) -> Vec<&'a str> {
        order.iter().map(|record| record.id()).collect()
    }

    #[test]
    fn the_held_record_moves_to_the_hovered_slot_in_its_own_list_only() {
        const CASES: &[Case] = &[
            ("agents", 2, ["b", "c", "a"]),
            ("agents", 9, ["b", "c", "a"]),
            ("agents", 0, ["a", "b", "c"]),
            ("hosts", 2, ["a", "b", "c"]),
        ];
        let records = ["a", "b", "c"].map(|id| Record::new(id, id));
        for &(source, over, expected) in CASES {
            let lift = lifted(source, over);
            let ids = ids(&arranged(Some(&lift), "agents", &records));
            assert_eq!(ids, expected, "{source} {over}");
        }
    }
}
