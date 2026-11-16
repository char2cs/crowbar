/// Doubles a value.
pub fn double(value: u32) -> u32 {
    value + value
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn doubles() {
        assert_eq!(double(1), 2);
    }
}
