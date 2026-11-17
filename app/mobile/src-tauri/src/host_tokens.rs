use std::sync::Arc;

use tauri::State;

use crate::host_error::HostError;

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
pub(crate) async fn forget_host_token(
    host_id: String,
    store: State<'_, Arc<dyn TokenStore>>,
) -> Result<(), HostError> {
    let store = Arc::clone(store.inner());
    tokio::task::spawn_blocking(move || store.delete(&host_id))
        .await
        .map_err(HostError::Interrupted)??;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::token_error::TokenError;
    use std::collections::HashMap;
    use std::sync::Mutex;
    use tauri::test::mock_app;
    use tauri::Manager;

    struct FakeStore(Mutex<HashMap<String, String>>);

    impl TokenStore for FakeStore {
        fn get(
            &self,
            host_id: &str,
        ) -> Result<Option<String>, TokenError> {
            Ok(self.0.lock().unwrap().get(host_id).cloned())
        }

        fn set(
            &self,
            host_id: &str,
            token: &str,
        ) -> Result<(), TokenError> {
            self.0.lock().unwrap().insert(host_id.into(), token.into());
            Ok(())
        }

        fn delete(
            &self,
            host_id: &str,
        ) -> Result<(), TokenError> {
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
        tauri::async_runtime::block_on(forget_host_token("a".into(), app.state())).unwrap();
        let store = app.state::<Arc<dyn TokenStore>>();
        assert_eq!(store.get("a").unwrap(), None);
        assert_eq!(store.get("b").unwrap(), Some("tb".to_owned()));
    }
}
