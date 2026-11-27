const LAST_SLOT: u8 = 9;

fn index_of(
    count: usize,
    slot: u8,
) -> Option<usize> {
    match slot {
        LAST_SLOT => count.checked_sub(1),
        _ => usize::from(slot).checked_sub(1),
    }
}

pub(crate) fn project_at<T>(
    projects: &[T],
    slot: u8,
) -> Option<&T> {
    projects.get(index_of(projects.len(), slot)?)
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Case {
        projects: &'static [&'static str],
        slot: u8,
        expected: Option<&'static str>,
    }

    #[test]
    fn a_slot_is_its_position_and_nine_is_the_last_project() {
        let three: &'static [&'static str] = &["a", "b", "c"];
        let case = |projects: &'static [&'static str], slot, expected| Case {
            projects,
            slot,
            expected,
        };
        let cases = [
            case(three, 1, Some("a")),
            case(three, 3, Some("c")),
            case(three, 4, None),
            case(three, 9, Some("c")),
            case(&[], 9, None),
            case(&[], 1, None),
            case(three, 0, None),
        ];
        for case in cases {
            let found = project_at(case.projects, case.slot).copied();
            assert_eq!(found, case.expected, "{}", case.slot);
        }
    }
}
