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
  // TEMP-DEBUG
  const dbg = (level, text) => crowbar.post(JSON.stringify({ sdk: 1, kind: 'dbg', level, text: String(text) }));
  for (const level of ['error', 'warn', 'log']) { const o = console[level]; console[level] = (...a) => { dbg(level, a.map(String).join(' ')); o.apply(console, a); }; }
  window.addEventListener('error', (e) => dbg('onerror', e.message + ' ' + e.filename + ':' + e.lineno));
  window.addEventListener('unhandledrejection', (e) => dbg('reject', e.reason));
  const chord = (event) => {
    const mac = navigator.platform.toLowerCase().includes('mac');
    const parts = [];
    if (mac ? event.metaKey : event.ctrlKey) parts.push('mod');
    if (event.shiftKey) parts.push('shift');
    if (event.altKey) parts.push('alt');
    parts.push(event.key.toLowerCase());
    return parts.join('+');
  };
  window.addEventListener(
    'keydown',
    (event) => {
      const pressed = chord(event);
      const hit = bindings.find((binding) => binding.keys === pressed);
      if (!hit) return;
      event.preventDefault();
      crowbar.post(JSON.stringify({ sdk: 1, kind: 'command.run', command: hit.command }));
    },
    true,
  );
  let hovered = false;
  const hover = (value) => {
    if (hovered === value) return;
    hovered = value;
    crowbar.post(JSON.stringify({ sdk: 1, kind: 'shell.hover', hovered: value }));
  };
  document.documentElement.addEventListener('mouseenter', () => hover(true));
  document.documentElement.addEventListener('mouseleave', () => hover(false));
  window.__crowbarReceive = (text) => {
    dbg('frame', text.slice(0, 300));
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
  setTimeout(() => { const cs = getComputedStyle(document.documentElement); dbg('state', document.documentElement.className + ' | ' + [...document.documentElement.attributes].map((a) => a.name + '=' + a.value).join(' ') + ' | chrome-bg=' + cs.getPropertyValue('--chrome-bg') + ' bg=' + cs.getPropertyValue('--background') + ' body=' + getComputedStyle(document.body).backgroundColor); }, 8000);
  setTimeout(() => { let el = document.getElementById('root'); const out = []; for (let i = 0; el && i < 8; i++) { const c = getComputedStyle(el); out.push(el.tagName + '.' + String(el.className).slice(0, 80) + ' bg=' + c.backgroundColor + ' ' + el.offsetWidth + 'x' + el.offsetHeight); el = el.firstElementChild; } dbg('tree', out.join(' >> ')); }, 9000);
  window.crowbar = crowbar;
})();
