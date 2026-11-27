use std::collections::HashSet;

#[derive(Default)]
pub(crate) struct FamilyNames {
    held: HashSet<&'static str>,
}

impl FamilyNames {
    pub(crate) fn intern(
        &mut self,
        name: &str,
    ) -> Option<&'static str> {
        if name.is_empty() {
            return None;
        }
        if let Some(known) = self.held.get(name) {
            return Some(known);
        }
        let kept: &'static str = Box::leak(name.to_owned().into_boxed_str());
        self.held.insert(kept);
        Some(kept)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_name_is_kept_once_and_an_empty_one_is_no_choice() {
        let mut names = FamilyNames::default();
        let first = names.intern("Inter").unwrap();
        let again = names.intern(&String::from("Inter")).unwrap();
        assert!(std::ptr::eq(first, again));
        assert_eq!(first, "Inter");
        assert_eq!(names.intern(""), None);
    }
}
