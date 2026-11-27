pub(crate) fn attr<'a>(
    markup: &'a str,
    name: &str,
) -> Option<&'a str> {
    let (_, rest) = markup.split_once(&format!(" {name}=\""))?;
    rest.split_once('"').map(|(value, _)| value)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_attribute_is_found_by_its_whole_name() {
        let tag = r#"<circle cx="3" cy="9" r="2" fill-opacity="0.15">"#;
        assert_eq!(attr(tag, "cy"), Some("9"));
        assert_eq!(attr(tag, "r"), Some("2"));
        assert_eq!(attr(tag, "opacity"), None);
    }
}
