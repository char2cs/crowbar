#[cfg(target_os = "ios")]
use std::collections::HashMap;
use std::error::Error;

use tauri::plugin::PluginApi;
use tauri::Runtime;

use crate::swift_plugin::SwiftPlugin;

#[cfg(target_os = "ios")]
tauri::ios_plugin_binding!(init_plugin_crowbar_chrome);

pub(crate) struct Registration<'a, R: Runtime>(pub(crate) &'a PluginApi<R, ()>);

// PLATFORM: tauri hands a command with no Rust handler to the Swift plugin, so there are no Rust commands:
// PLATFORM: tauri payload and channels stay intact, where a Rust wrapper would re-key the payload.
#[cfg(target_os = "ios")]
impl<R: Runtime> SwiftPlugin for Registration<'_, R> {
    fn register(
        &self,
        frontend: usize,
    ) -> Result<(), Box<dyn Error>> {
        let plugin = self.0.register_ios_plugin(init_plugin_crowbar_chrome)?;
        plugin
            .run_mobile_plugin::<()>("attachFrontend", HashMap::from([("frontend", frontend)]))?;
        Ok(())
    }
}

#[cfg(not(target_os = "ios"))]
impl<R: Runtime> SwiftPlugin for Registration<'_, R> {
    fn register(
        &self,
        _frontend: usize,
    ) -> Result<(), Box<dyn Error>> {
        let Self(_api) = self;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use tauri::test::MockRuntime;

    use super::*;

    fn is_a_swift_plugin<P: SwiftPlugin>() {}

    #[test]
    fn the_registration_is_a_swift_plugin() {
        is_a_swift_plugin::<Registration<'_, MockRuntime>>();
    }
}
