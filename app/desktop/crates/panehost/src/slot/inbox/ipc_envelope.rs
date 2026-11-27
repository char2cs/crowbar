pub(crate) fn open_envelope(text: &str) -> Option<(u64, &str)> {
    let (generation, frame) = text.split_once(':')?;
    Some((generation.parse().ok()?, frame))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn splits_the_generation_from_the_frame() {
        assert_eq!(
            open_envelope(r#"7:{"a":1}"#),
            Some((7, r#"{"a":1}"#)),
            "wrapped frame"
        );
    }

    #[test]
    fn anything_else_is_not_an_envelope() {
        assert_eq!(open_envelope("{}"), None, "no generation");
        assert_eq!(
            open_envelope("null:{}"),
            None,
            "a page without a generation"
        );
    }
}
