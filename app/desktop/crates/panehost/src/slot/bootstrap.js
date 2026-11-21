(() => {
  const queue = [];
  let bindings = [];
  const generation = new URLSearchParams(window.location.search).get('g') || '0';
  // Panes read their relay endpoint synchronously, so it is fetched before any page script runs.
  try {
    const lookup = new XMLHttpRequest();
    lookup.open('GET', window.__CROWBAR__.api + '/__crowbar/relay', false);
    lookup.send();
    if (lookup.status === 200) window.__CROWBAR__.ws = JSON.parse(lookup.responseText);
  } catch (error) {
    console.error('crowbar: relay lookup failed', error);
  }
  const crowbar = {
    post: (text) => window.ipc.postMessage(generation + ':' + text),
    onmessage: null,
  };
  const chord = (event) => {
    const mac = navigator.platform.toLowerCase().includes('mac');
    const parts = [];
    if (mac ? event.metaKey : event.ctrlKey) parts.push('mod');
    if (event.shiftKey) parts.push('shift');
    if (event.altKey) parts.push('alt');
    // Shift turns a digit into a symbol, so the physical digit key names the chord.
    const digit = /^Digit([0-9])$/.exec(event.code);
    parts.push(digit ? digit[1] : event.key.toLowerCase());
    return parts.join('+');
  };
  // Bubble phase, as develop's own shortcuts: a page control that handles the chord (bold in the
  // composer) stops it before the shell sees it.
  window.addEventListener('keydown', (event) => {
    const pressed = chord(event);
    const hit = bindings.find((binding) => binding.keys === pressed);
    if (!hit) return;
    // A page-first binding (Escape on a drawer, Mod+F on the Files tree) yields to a page that
    // handled the key, as develop's own handlers check defaultPrevented.
    if (hit.precedence === 'page' && event.defaultPrevented) return;
    event.preventDefault();
    crowbar.post(JSON.stringify({ sdk: 1, kind: 'command.run', command: hit.command }));
  });
  let hovered = false;
  const hover = (value) => {
    if (hovered === value) return;
    hovered = value;
    crowbar.post(JSON.stringify({ sdk: 1, kind: 'shell.hover', hovered: value }));
  };
  document.documentElement.addEventListener('mouseenter', () => hover(true));
  document.documentElement.addEventListener('mouseleave', () => hover(false));
  // A press, not window focus: the shell's own hand-offs focus the window, and echoing them back
  // would overturn the shell's later choice to keep the keyboard.
  window.addEventListener(
    'pointerdown',
    () => crowbar.post(JSON.stringify({ sdk: 1, kind: 'shell.focus' })),
    true,
  );
  window.__crowbarReceive = (text) => {
    try {
      const frame = JSON.parse(text);
      if (frame.kind === 'keybindings') bindings = frame.bindings;
    } catch (error) {
      console.error('crowbar: unreadable frame', error);
    }
    if (crowbar.onmessage) crowbar.onmessage(text);
    else queue.push(text);
  };
  Object.defineProperty(crowbar, 'onmessage', {
    get: () => crowbar._handler || null,
    set: (handler) => {
      crowbar._handler = handler;
      while (handler && queue.length) handler(queue.shift());
    },
  });
  window.crowbar = crowbar;
})();
