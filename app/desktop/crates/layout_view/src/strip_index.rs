pub(crate) fn insertion_index(
    target: usize,
    dragged: Option<usize>,
) -> usize {
    match dragged {
        Some(from) if from < target => target - 1,
        _ => target,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (&'static str, usize, Option<usize>, usize);

    const CASES: &[Case] = &[
        ("from another strip lands before the target", 2, None, 2),
        ("from before the target lands one earlier", 2, Some(0), 1),
        ("from after the target lands at the target", 1, Some(3), 1),
        ("on its own place stays", 2, Some(2), 2),
        ("to the front", 0, Some(2), 0),
    ];

    #[test]
    fn a_tab_dropped_before_another_lands_among_the_others() {
        for (name, target, dragged, expected) in CASES {
            assert_eq!(insertion_index(*target, *dragged), *expected, "{name}");
        }
    }
}
