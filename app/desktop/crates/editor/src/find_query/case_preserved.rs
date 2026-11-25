// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/base/common/search.ts (buildReplaceStringWithCasePreserved)
const SEPARATORS: [char; 2] = ['-', '_'];

pub(crate) fn case_preserved(
    matched: &str,
    pattern: &str,
) -> String {
    if matched.is_empty() {
        return pattern.to_owned();
    }
    let alike: Vec<char> = SEPARATORS
        .into_iter()
        .filter(|separator| splits_alike(matched, pattern, *separator))
        .collect();
    if let [separator] = alike.as_slice() {
        return by_parts(matched, pattern, *separator);
    }
    cased_like(matched, pattern)
}

fn splits_alike(
    matched: &str,
    pattern: &str,
    separator: char,
) -> bool {
    let both = matched.contains(separator) && pattern.contains(separator);
    both && matched.split(separator).count() == pattern.split(separator).count()
}

fn by_parts(
    matched: &str,
    pattern: &str,
    separator: char,
) -> String {
    let parts: Vec<String> = matched
        .split(separator)
        .zip(pattern.split(separator))
        .map(|(matched, pattern)| case_preserved(matched, pattern))
        .collect();
    parts.join(&separator.to_string())
}

fn cased_like(
    matched: &str,
    pattern: &str,
) -> String {
    if matched.to_uppercase() == matched {
        return pattern.to_uppercase();
    }
    if matched.to_lowercase() == matched {
        return pattern.to_lowercase();
    }
    let mut rest = pattern.chars();
    let (Some(first), Some(head)) = (matched.chars().next(), rest.next()) else {
        return pattern.to_owned();
    };
    if first.is_uppercase() {
        return head.to_uppercase().chain(rest).collect();
    }
    if first.is_lowercase() {
        return head.to_lowercase().chain(rest).collect();
    }
    pattern.to_owned()
}

#[cfg(test)]
mod tests {
    use super::*;

    // SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/base/test/common/search.test.ts
    const CASES: &[(&str, &str, &str)] = &[
        ("abc", "Def", "def"),
        ("Abc", "Def", "Def"),
        ("ABC", "Def", "DEF"),
        ("aBc", "Def", "def"),
        ("AbC", "def", "Def"),
        ("Foo-Bar", "newfoo-newbar", "Newfoo-Newbar"),
        (
            "Foo-Bar-Abc",
            "newfoo-newbar-newabc",
            "Newfoo-Newbar-Newabc",
        ),
        ("Foo-Bar-abc", "newfoo-newbar", "Newfoo-newbar"),
        ("foo-Bar", "newfoo-newbar", "newfoo-Newbar"),
        ("Foo_Bar", "newfoo_newbar", "Newfoo_Newbar"),
        ("FOO_BAR", "newfoo_newbar", "NEWFOO_NEWBAR"),
        (
            "Foo-Bar_Abc",
            "newfoo-newbar_newabc",
            "Newfoo-newbar_newabc",
        ),
        ("", "Def", "Def"),
    ];

    #[test]
    fn a_pattern_takes_the_case_of_the_match_part_by_part_as_monaco_s_does() {
        for (matched, pattern, expected) in CASES {
            assert_eq!(case_preserved(matched, pattern), *expected, "{matched:?}");
        }
    }
}
