//! The id a paired host is registered under.

use crate::new_id;

const SUFFIX_LEN: usize = 8;
const FALLBACK: &str = "host";

fn slug(name: &str) -> String {
    let mapped: String = name
        .trim()
        .to_lowercase()
        .chars()
        .map(|ch| Some(ch).filter(char::is_ascii_alphanumeric).unwrap_or('-'))
        .collect();
    let collapsed = mapped.split('-').filter(|part| !part.is_empty());
    collapsed.collect::<Vec<_>>().join("-")
}

pub(crate) fn of(name: &str) -> String {
    let slug = Some(slug(name))
        .filter(|slug| !slug.is_empty())
        .unwrap_or_else(|| FALLBACK.to_owned());
    let suffix: String = new_id::uuid().chars().take(SUFFIX_LEN).collect();
    format!("{slug}-{suffix}")
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, &str); 4] = [
        ("Home Lab", "home-lab-"),
        ("  --a__b--  ", "a-b-"),
        ("", "host-"),
        ("Ünï", "n-"),
    ];

    #[test]
    fn a_name_becomes_a_slug_with_a_random_suffix() {
        for (name, prefix) in CASES {
            let id = of(name);
            assert!(id.starts_with(prefix), "{name:?} gave {id}");
            assert_eq!(id.len(), prefix.len() + SUFFIX_LEN, "{id}");
            assert!(
                id.bytes()
                    .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
            );
        }
        assert_ne!(of("a"), of("a"));
    }
}
