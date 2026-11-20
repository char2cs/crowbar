use gpui::SharedString;

pub(crate) const HEIGHT: f32 = 32.0;

pub(crate) fn group_of(name: &str) -> SharedString {
    SharedString::from(format!("strip-tab-{name}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_tab_hovers_as_a_group_of_its_own() {
        assert_ne!(group_of("a"), group_of("b"), "one hover group per pane");
    }
}
