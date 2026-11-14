(() => {
  const queue = [];
  let bindings = [];
  const crowbar = {
    post: (text) => window.ipc.postMessage(text),
    onmessage: null,
  };
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
      const hit = bindings.find((binding) => binding.keys === chord(event));
      if (!hit) return;
      event.preventDefault();
      crowbar.post(JSON.stringify({ sdk: 1, kind: 'command.run', command: hit.command }));
    },
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
