use std::sync::Arc;

use tauri::State;

mod credential;
mod keychain;
mod keychain_token_store;
mod keyring_token_store;
mod os_keychain;
mod read_error;
mod token_store;

pub(crate) use credential::Credential;
pub(crate) use keyring_token_store::KeyringTokenStore;
pub(crate) use token_store::TokenStore;

#[tauri::command]
#[expect(
    clippy::needless_pass_by_value,
    reason = "tauri injects managed state by value"
)]
pub(crate) fn forget_host_token(
    host_id: &str,
    store: State<'_, Arc<dyn TokenStore>>,
) -> Result<(), String> {
    store.delete(host_id)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;
    use std::sync::Mutex;
    use tauri::test::mock_app;
    use tauri::Manager;

    struct FakeStore(Mutex<HashMap<String, String>>);

    impl TokenStore for FakeStore {
        fn get(
            &self,
            host_id: &str,
        ) -> Result<Option<String>, String> {
            Ok(self.0.lock().unwrap().get(host_id).cloned())
        }

        fn set(
            &self,
            host_id: &str,
            token: &str,
        ) -> Result<(), String> {
            self.0.lock().unwrap().insert(host_id.into(), token.into());
            Ok(())
        }

        fn delete(
            &self,
            host_id: &str,
        ) -> Result<(), String> {
            self.0.lock().unwrap().remove(host_id);
            Ok(())
        }
    }

    #[test]
    fn forgetting_a_host_removes_only_its_token() {
        let app = mock_app();
        let store = FakeStore(Mutex::default());
        store.set("a", "ta").unwrap();
        store.set("b", "tb").unwrap();
        app.manage(Arc::new(store) as Arc<dyn TokenStore>);
        forget_host_token("a", app.state()).unwrap();
        let store = app.state::<Arc<dyn TokenStore>>();
        assert_eq!(store.get("a").unwrap(), None);
        assert_eq!(store.get("b").unwrap(), Some("tb".to_owned()));
    }
}
