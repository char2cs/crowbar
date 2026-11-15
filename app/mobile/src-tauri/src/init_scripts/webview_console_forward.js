(function () {
  function send(level, parts) {
    try {
      var message = parts.map(function (p) {
        return p instanceof Error ? p.stack || String(p) : typeof p === 'string' ? p : JSON.stringify(p);
      }).join(' ');
      window.__TAURI_INTERNALS__.invoke('plugin:log|log', { level: level, message: message, location: 'webview' });
    } catch (_) {}
  }
  ['warn', 'error'].forEach(function (name) {
    var original = console[name];
    console[name] = function () {
      send(name === 'error' ? 5 : 4, Array.prototype.slice.call(arguments));
      original.apply(console, arguments);
    };
  });
  window.addEventListener('error', function (e) { send(5, ['uncaught', e.message, e.filename + ':' + e.lineno]); });
  window.addEventListener('unhandledrejection', function (e) { send(5, ['unhandledrejection', e.reason]); });
})();
