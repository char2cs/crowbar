// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/find/browser/replacePattern.ts ($n, $& and $$ in a replace string)
const DOLLAR: char = '$';
const WHOLE_MATCH: char = '&';

pub(crate) fn rust_template(monaco: &str) -> String {
    let mut chars = monaco.chars().peekable();
    let template: String = std::iter::from_fn(|| piece(&mut chars)).collect();
    template.replace("\\n", "\n").replace("\\t", "\t")
}

fn piece(chars: &mut std::iter::Peekable<std::str::Chars<'_>>) -> Option<String> {
    let character = chars.next()?;
    if character != DOLLAR {
        return Some(character.to_string());
    }
    Some(dollar(chars))
}

fn dollar(chars: &mut std::iter::Peekable<std::str::Chars<'_>>) -> String {
    if chars.next_if_eq(&DOLLAR).is_some() {
        return "$$".to_owned();
    }
    if chars.next_if_eq(&WHOLE_MATCH).is_some() {
        return "${0}".to_owned();
    }
    let digits: String = std::iter::from_fn(|| chars.next_if(char::is_ascii_digit)).collect();
    if digits.is_empty() {
        return "$$".to_owned();
    }
    format!("${{{digits}}}")
}

#[cfg(test)]
mod tests {
    use super::rust_template;

    const TEMPLATES: &[(&str, &str)] = &[
        ("$1x", "${1}x"),
        ("[$&]", "[${0}]"),
        ("$$", "$$"),
        ("$x", "$$x"),
        ("a\\nb", "a\nb"),
    ];

    #[test]
    fn a_monaco_replace_string_becomes_the_regex_crate_s_template() {
        for (monaco, rust) in TEMPLATES {
            assert_eq!(rust_template(monaco), *rust, "{monaco:?}");
        }
    }
}
