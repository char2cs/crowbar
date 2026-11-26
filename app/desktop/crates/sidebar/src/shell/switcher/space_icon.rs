//! The mark of a space.

mod icon_choice;

/// What a space shows as its icon.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub enum SpaceIcon {
    /// The generated default mark.
    #[default]
    Default,
    /// An emoji of up to four characters.
    Emoji(String),
    /// An image file on this computer (png, jpeg or webp), uploaded to the daemon.
    Image(String),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_space_has_the_default_mark_until_one_is_set() {
        assert_eq!(SpaceIcon::default(), SpaceIcon::Default);
        assert_ne!(SpaceIcon::Emoji("x".into()), SpaceIcon::Image("x".into()));
    }
}
