use sidebar::{AvatarTint, Mark};

const TINTS: [(&str, AvatarTint); 8] = [
    ("rose", AvatarTint::Rose),
    ("amber", AvatarTint::Amber),
    ("emerald", AvatarTint::Emerald),
    ("cyan", AvatarTint::Cyan),
    ("indigo", AvatarTint::Indigo),
    ("violet", AvatarTint::Violet),
    ("slate", AvatarTint::Slate),
    ("pink", AvatarTint::Pink),
];

pub(crate) fn of(
    (name, label): (&str, &str),
    (emoji, color): (&str, &str),
) -> Mark {
    if !emoji.is_empty() {
        return Mark::Emoji(emoji.to_owned());
    }
    let letter = [label, name]
        .iter()
        .find_map(|text| text.chars().next())
        .map_or('R', |first| first.to_ascii_uppercase());
    let tint = TINTS
        .iter()
        .find(|(word, _)| color.ends_with(word))
        .map_or_else(AvatarTint::default, |(_, tint)| *tint);
    Mark::Letter { letter, tint }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_emoji_wins_over_the_letter() {
        assert_eq!(
            of(("repo", "R"), ("🦀", "avatar-rose")),
            Mark::Emoji("🦀".into())
        );
    }

    #[test]
    fn the_letter_tile_takes_the_label_then_the_name_and_the_daemons_tint() {
        let mark = of(("repo", "x"), ("", "avatar-violet"));
        assert_eq!(
            mark,
            Mark::Letter {
                letter: 'X',
                tint: AvatarTint::Violet
            }
        );
        let mark = of(("repo", ""), ("", "unknown"));
        assert_eq!(
            mark,
            Mark::Letter {
                letter: 'R',
                tint: AvatarTint::Indigo
            }
        );
    }
}
