use store::ImportPlan;

fn branches(count: usize) -> String {
    if count == 1 {
        return "1 branch".to_owned();
    }
    format!("{count} branches")
}

fn parents(count: usize) -> String {
    if count == 1 {
        return "1 parent branch".to_owned();
    }
    format!("{count} parent branches")
}

pub(crate) fn of(plan: ImportPlan) -> String {
    if plan.parent_count == 0 {
        return format!("Imports {}", branches(plan.import_count));
    }
    format!(
        "Imports {} · creates {}",
        branches(plan.import_count),
        parents(plan.parent_count)
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    fn plan(
        import_count: usize,
        parent_count: usize,
    ) -> ImportPlan {
        ImportPlan {
            import_count,
            parent_count,
        }
    }

    const CASES: [(usize, usize, &str); 4] = [
        (0, 0, "Imports 0 branches"),
        (1, 0, "Imports 1 branch"),
        (3, 1, "Imports 3 branches · creates 1 parent branch"),
        (1, 2, "Imports 1 branch · creates 2 parent branches"),
    ];

    #[test]
    fn the_hint_counts_branches_and_the_parents_it_creates() {
        for (import_count, parent_count, expected) in CASES {
            assert_eq!(of(plan(import_count, parent_count)), expected);
        }
    }
}
