//! Window titles set by the program in the terminal.

const ESC: char = '\u{1b}';

/// A title safe to show, or an empty string for "no update".
///
/// A title holding ESC is a prompt's coloured text leaking in; leftover CSI bodies (`[01;32m`) after
/// control bytes are stripped are rejected too, while `[WIP]` survives.
pub fn sanitize_title(raw: &str) -> String {
    if raw.contains(ESC) {
        return String::new();
    }
    let stripped: String = raw
        .chars()
        .filter(|character| !character.is_control())
        .collect();
    let trimmed = stripped.trim();
    if has_csi_body(trimmed) {
        return String::new();
    }
    trimmed.to_owned()
}

fn has_csi_body(text: &str) -> bool {
    text.match_indices('[')
        .any(|(index, _)| ends_csi(text.get(index + 1..).unwrap_or_default()))
}

fn ends_csi(rest: &str) -> bool {
    let body = rest.trim_start_matches(is_csi_parameter);
    body.chars()
        .next()
        .is_some_and(|last| "mhlKJH".contains(last))
}

fn is_csi_parameter(character: char) -> bool {
    character.is_ascii_digit() || character == ';' || character == '?'
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn titles_are_cleaned_or_rejected() {
        let cases = [
            ("vim main.rs", "vim main.rs"),
            ("  padded \u{7}", "padded"),
            ("\u{1b}[32muser@host", ""),
            ("[01;32muser@host", ""),
            ("[WIP] fix", "[WIP] fix"),
            ("", ""),
        ];
        for (raw, expected) in cases {
            assert_eq!(sanitize_title(raw), expected, "{raw:?}");
        }
    }
}
