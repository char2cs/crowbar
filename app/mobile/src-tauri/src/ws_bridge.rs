use std::sync::Arc;
use std::time::Duration;

use tauri::ipc::Request;
use tauri::{Manager, Runtime, State, Webview};
use tokio_tungstenite::tungstenite::Message;

use crate::host_error::HostError;
use crate::host_route::is_remote_host;
use crate::host_tokens::TokenStore;
use crate::hosts::HostRegistry;

mod attach;
mod connection;
pub(crate) mod frame;
pub(crate) mod frame_sink;
mod open_args;
mod open_request;
mod paired_host;
mod remote;
mod session;
mod upgrade;
mod writer;
mod ws_bridge_manager;

use open_args::OpenArgs;
use open_request::OpenRequest;
pub(crate) use ws_bridge_manager::WsBridgeManager;

fn remote_host(host_id: Option<String>) -> Option<String> {
    host_id.filter(|host| is_remote_host(host))
}

async fn open_remote<R: Runtime>(
    args: OpenArgs,
    webview: &Webview<R>,
) -> Result<(), HostError> {
    let host_id = remote_host(args.host_id.clone()).ok_or(HostError::NoLocalDaemon)?;
    let on_message = args.channel(webview)?;
    let registry = webview.state::<HostRegistry>();
    let tokens = webview.state::<Arc<dyn TokenStore>>();
    let paired = paired_host::paired_host(&registry, &tokens, &host_id)
        .await
        .inspect_err(|e| log::error!("ws_open: remote host {host_id}: {e}"))?;
    let request = OpenRequest {
        conn_id: args.conn_id,
        path: args.path,
        host_id: None,
        window_label: webview.label().to_owned(),
        read_idle_timeout: args.idle_timeout_ms.map(Duration::from_millis),
    };
    let manager = webview.state::<WsBridgeManager>();
    remote::open_remote_bridge(paired, on_message, &manager, request)
        .await
        .map(drop)
}

#[tauri::command]
pub(crate) async fn ws_open<R: Runtime>(
    request: Request<'_>,
    webview: Webview<R>,
) -> Result<(), HostError> {
    open_remote(OpenArgs::from_request(&request)?, &webview).await
}

#[tauri::command]
pub(crate) async fn ws_send(
    conn_id: String,
    data: String,
    manager: State<'_, WsBridgeManager>,
) -> Result<(), HostError> {
    let Some(tx) = manager.sender(&conn_id) else {
        return Err(HostError::NoWsConnection(conn_id));
    };
    if tx.send(Message::Text(data.into())).is_err() {
        return Err(HostError::WsClosed(conn_id));
    }
    Ok(())
}

#[tauri::command]
pub(crate) async fn ws_close(
    conn_id: String,
    manager: State<'_, WsBridgeManager>,
) -> Result<(), HostError> {
    manager.retire(&conn_id);
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ws_bridge::connection::Connection;
    use tauri::test::{mock_builder, mock_context, noop_assets, MockRuntime};
    use tokio::sync::{mpsc, oneshot};

    #[test]
    fn only_a_non_localhost_host_id_opens_a_remote_bridge() {
        type Case = (Option<&'static str>, Option<&'static str>);
        const CASES: &[Case] = &[
            (None, None),
            (Some(""), None),
            (Some("localhost"), None),
            (Some("host-1"), Some("host-1")),
        ];
        for (given, expected) in CASES {
            let host = remote_host(given.map(str::to_owned));
            assert_eq!(host.as_deref(), *expected, "{given:?}");
        }
    }

    fn app_with_connection(
        conn_id: &str
    ) -> (tauri::App<MockRuntime>, mpsc::UnboundedReceiver<Message>) {
        let app = mock_builder()
            .manage(WsBridgeManager::default())
            .build(mock_context(noop_assets()))
            .unwrap();
        let (tx, queue) = mpsc::unbounded_channel();
        let (cancel, _cancelled) = oneshot::channel();
        let connection = Connection {
            generation: 0,
            window_label: "main".to_owned(),
            host_id: None,
            tx,
            _cancel: cancel,
        };
        app.state::<WsBridgeManager>()
            .register(conn_id.to_owned(), connection);
        (app, queue)
    }

    #[tokio::test]
    async fn ws_send_reaches_an_open_connection_and_names_a_missing_one() {
        let (app, mut queue) = app_with_connection("c1");
        let sent = ws_send("c1".into(), "{}".into(), app.state()).await;
        assert!(sent.is_ok());
        assert_eq!(queue.recv().await, Some(Message::Text("{}".into())));
        let missing = ws_send("c2".into(), "{}".into(), app.state()).await;
        assert_eq!(missing.unwrap_err().to_string(), "no open ws connection c2");
    }

    #[tokio::test]
    async fn ws_close_retires_the_connection_and_a_later_send_fails() {
        let (app, _queue) = app_with_connection("c1");
        assert!(ws_close("c1".into(), app.state()).await.is_ok());
        let sent = ws_send("c1".into(), "{}".into(), app.state()).await;
        assert_eq!(sent.unwrap_err().to_string(), "no open ws connection c1");
    }
}
