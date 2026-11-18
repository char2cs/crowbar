//! Runs a table of named scenarios and says which one failed.

pub(crate) type Case = (&'static str, fn());

pub(crate) fn run_cases(cases: &[Case]) {
    for &(name, case) in cases {
        assert!(
            std::panic::catch_unwind(case).is_ok(),
            "case failed: {name}"
        );
    }
}
