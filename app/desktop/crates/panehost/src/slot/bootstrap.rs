use protocol::HostId;

const CHANNEL_SCRIPT: &str = include_str!("bootstrap.js");

pub(crate) fn bootstrap_script(
    scheme: &str,
    host: &HostId,
) -> String {
    let config = serde_json::json!({
        "api": format!("{scheme}://{host}"),
        "host": host.as_str(),
    });
    format!("window.__CROWBAR__ = {config};\n{CHANNEL_SCRIPT}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn config_names_the_api_base_and_host_and_the_channel_follows() {
        let host = HostId::new("local").unwrap();
        let script = bootstrap_script("crowbar", &host);
        assert!(
            script.starts_with(r#"window.__CROWBAR__ = {"api":"crowbar://local","host":"local"};"#),
            "{script}"
        );
        assert!(
            script.contains("__crowbarReceive"),
            "channel script is appended"
        );
        assert!(script.contains("generation + ':' + text"), "envelope");
        assert!(script.contains("/__crowbar/relay"), "relay lookup");
    }

    #[cfg(target_os = "linux")]
    use std::sync::{Arc, mpsc};
    #[cfg(target_os = "linux")]
    use std::time::Duration;

    #[cfg(target_os = "linux")]
    use webview::{
        Bounds, Config, Host, HostConfig, Kind, ParentWindow, ProtocolHandler, ProtocolRequest,
        ProtocolResponder, ProtocolResponse, Visibility,
    };
    #[cfg(target_os = "linux")]
    use x11rb::COPY_DEPTH_FROM_PARENT;
    #[cfg(target_os = "linux")]
    use x11rb::connection::Connection as _;
    #[cfg(target_os = "linux")]
    use x11rb::protocol::xproto::{ConnectionExt as _, CreateWindowAux, WindowClass};
    #[cfg(target_os = "linux")]
    use x11rb::rust_connection::RustConnection;

    #[cfg(target_os = "linux")]
    type TestResult<T = ()> = Result<T, Box<dyn std::error::Error>>;

    #[cfg(target_os = "linux")]
    const PAGE: &str = "<input id=composer><div id=menu tabindex=0></div><script>
composer.addEventListener('keydown', (event) => event.stopPropagation());
menu.addEventListener('keydown', (event) => event.preventDefault());
addEventListener('load', () => window.ipc.postMessage('ready'));
addEventListener('focus', () => window.ipc.postMessage('focused'));
addEventListener('blur', () => window.ipc.postMessage('blurred'));
</script>";
    #[cfg(target_os = "linux")]
    const BINDINGS: &str = "__crowbarReceive(JSON.stringify({ kind: 'keybindings', bindings: [
  { keys: 'mod+b', command: 'navigation.toggleSidebar' },
  { keys: 'mod+t', command: 'tabs.new' },
  { keys: 'mod+j', command: 'tabs.newTerminal' },
  { keys: 'mod+f', command: 'files.search', precedence: 'page' },
  { keys: 'escape', command: 'sidebar.dismissDrawer', precedence: 'page' },
] }))";
    #[cfg(target_os = "linux")]
    const PRESSES: &str = "
const press = (target, key, ctrlKey = true) => target.dispatchEvent(
  new KeyboardEvent('keydown', { key, ctrlKey, bubbles: true, cancelable: true }));
press(composer, 'b');
press(document.body, 't');
press(menu, 'j');
press(menu, 'f');
press(document.body, 'Escape', false);
window.ipc.postMessage('done');";
    #[cfg(target_os = "linux")]
    const PRESS: &str =
        "document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
window.ipc.postMessage('pressed');";
    #[cfg(target_os = "linux")]
    const WAIT: Duration = Duration::from_secs(30);
    #[cfg(target_os = "linux")]
    const SIDE: u16 = 100;

    #[cfg(target_os = "linux")]
    struct Page;

    #[cfg(target_os = "linux")]
    impl ProtocolHandler for Page {
        fn handle(
            &self,
            _request: ProtocolRequest,
            respond: ProtocolResponder,
        ) {
            respond(ProtocolResponse {
                status: 200,
                headers: vec![("content-type".into(), "text/html".into())],
                body: PAGE.as_bytes().to_vec(),
            });
        }
    }

    #[cfg(target_os = "linux")]
    fn parent_window() -> TestResult<(RustConnection, u32)> {
        let (connection, screen) = RustConnection::connect(None)?;
        let root = connection.setup().roots[screen].root;
        let window = connection.generate_id()?;
        connection.create_window(
            COPY_DEPTH_FROM_PARENT,
            window,
            root,
            0,
            0,
            SIDE,
            SIDE,
            0,
            WindowClass::INPUT_OUTPUT,
            0,
            &CreateWindowAux::new(),
        )?;
        connection.map_window(window)?;
        connection.get_input_focus()?.reply()?;
        Ok((connection, window))
    }

    #[cfg(target_os = "linux")]
    fn heard_until(
        messages: &mpsc::Receiver<String>,
        last: &str,
    ) -> Vec<String> {
        std::iter::from_fn(|| messages.recv_timeout(WAIT).ok())
            .take_while(|message| message != last)
            .collect()
    }

    #[cfg(target_os = "linux")]
    fn assert_ran_only_what_the_page_left_alone(heard: &[String]) {
        let ran = |command: &str| heard.iter().any(|message| message.contains(command));
        assert!(
            !ran("navigation.toggleSidebar"),
            "the shell took the composer's key: {heard:?}"
        );
        assert!(ran("tabs.new"), "the shell missed a free chord: {heard:?}");
        assert!(
            ran("tabs.newTerminal"),
            "a shell-first chord yielded to a prevented key: {heard:?}"
        );
        assert!(
            !ran("files.search"),
            "a page-first chord ran over a prevented key: {heard:?}"
        );
        assert!(
            ran("sidebar.dismissDrawer"),
            "a page-first chord the page left alone did not run: {heard:?}"
        );
    }

    #[cfg(target_os = "linux")]
    fn bootstrapped_page(
        host: &Host,
        parent: u32,
    ) -> TestResult<(webview::Handle, mpsc::Receiver<String>)> {
        let (sender, messages) = mpsc::channel();
        let side = f64::from(SIDE);
        let bounds = Bounds {
            x: 0.0,
            y: 0.0,
            width: side,
            height: side,
        };
        let config = Config {
            url: "crowbar://test/page".into(),
            bounds,
            kind: Kind::Core,
            visibility: Visibility::Shown,
            background: None,
            initialization_script: Some(bootstrap_script("crowbar", &HostId::new("local")?)),
            on_ipc: Arc::new(move |text| sender.send(text).unwrap_or_default()),
            on_page: None,
        };
        let view = host.create_in(ParentWindow::X11(parent), config)?;
        heard_until(&messages, "ready");
        Ok((view, messages))
    }

    #[cfg(target_os = "linux")]
    fn a_page_that_handles_a_chord_keeps_it_from_the_shell(
        host: &Host,
        parent: u32,
    ) -> TestResult {
        let (view, messages) = bootstrapped_page(host, parent)?;
        view.eval(BINDINGS)?;
        view.eval(PRESSES)?;
        let heard = heard_until(&messages, "done");
        assert_ran_only_what_the_page_left_alone(&heard);
        Ok(())
    }

    #[cfg(target_os = "linux")]
    fn only_a_press_on_the_page_reports_that_it_took_the_keyboard(
        host: &Host,
        parent: u32,
    ) -> TestResult {
        let (view, messages) = bootstrapped_page(host, parent)?;
        let reported = |last: &str| {
            heard_until(&messages, last)
                .iter()
                .any(|message| message.contains("shell.focus"))
        };
        view.focus()?;
        assert!(
            !reported("focused"),
            "the shell handing the page the keyboard"
        );
        view.focus_parent()?;
        assert!(!reported("blurred"), "the shell taking the keyboard back");
        view.eval(PRESS)?;
        assert!(reported("pressed"), "a press on the page");
        Ok(())
    }

    #[cfg(target_os = "linux")]
    #[test]
    #[ignore = "needs display"]
    fn bootstrapped_pages_on_x11() -> TestResult {
        let (_connection, parent) = parent_window()?;
        let host = Host::native(HostConfig {
            scheme: "crowbar".into(),
            protocol: Arc::new(Page),
            data_dir: None,
        })?;
        a_page_that_handles_a_chord_keeps_it_from_the_shell(&host, parent)?;
        only_a_press_on_the_page_reports_that_it_took_the_keyboard(&host, parent)
    }
}
