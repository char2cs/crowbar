use std::error::Error;

// PLATFORM: ios starts the Swift half here; a trait keeps the same signature on other targets.
pub(crate) trait SwiftPlugin {
    fn register(
        &self,
        frontend: usize,
    ) -> Result<(), Box<dyn Error>>;
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;

    use super::*;

    struct Counting(Cell<usize>);

    impl SwiftPlugin for Counting {
        fn register(
            &self,
            frontend: usize,
        ) -> Result<(), Box<dyn Error>> {
            self.0.set(frontend);
            Ok(())
        }
    }

    #[test]
    fn registration_hands_the_frontend_address_to_the_plugin() {
        let plugin = Counting(Cell::new(0));
        let object: &dyn SwiftPlugin = &plugin;
        object.register(7).unwrap();
        assert_eq!(plugin.0.get(), 7);
    }
}
