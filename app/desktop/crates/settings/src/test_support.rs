//! Fixtures shared by the unit tests: table cases and a state over the built-in schema.
#![cfg(test)]

pub(crate) type Case = (&'static str, fn());

pub(crate) fn run_cases(cases: &[Case]) {
    for &(name, case) in cases {
        assert!(
            std::panic::catch_unwind(case).is_ok(),
            "case failed: {name}"
        );
    }
}
