#![cfg(test)]

pub(crate) fn first(values: &[u32]) -> u32 {
    *values.first().expect("a value")
}
