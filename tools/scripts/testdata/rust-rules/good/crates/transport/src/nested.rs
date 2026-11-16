mod leaf;

/// The base value, built from the leaf.
pub fn base_value() -> u32 {
    leaf::double(2)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn doubles_through_the_leaf() {
        assert_eq!(base_value(), 4);
    }
}
