mod icon_choice;

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub enum SpaceIcon {
    #[default]
    Default,
    Emoji(String),
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
