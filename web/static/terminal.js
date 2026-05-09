'use strict';

/* ── Config ──────────────────────────────────────────────────────────────── */
const POLL_TIMEOUT_MS   = 20_000; // matches server long-poll timeout
const HEARTBEAT_MS      = 15_000;
const RECONNECT_MAX_MS  = 8_000;

/* ── Utilities ───────────────────────────────────────────────────────────── */
function b64encode(str) {
  // Encode a JS string (which may contain Unicode) to base64 bytes
  const bytes = new TextEncoder().encode(str);
  let bin = '';
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

function b64encodeBytes(bytes) {
  let bin = '';
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}

function b64decode(b64) {
  if (!b64) return new Uint8Array(0);
  const bin = atob(b64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

async function apiFetch(path, opts = {}) {
  const res = await fetch(path, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
    ...opts,
  });
  if (res.status === 401) {
    window.location.replace('/login');
    throw new Error('unauthenticated');
  }
  return res;
}

/* ── Tab / Terminal Manager ──────────────────────────────────────────────── */
let tabCounter = 0;
const tabs = []; // { id, sessionId, term, fitAddon, pane, tabEl, offset, pollCtrl, heartbeatTimer }

const tabBar   = document.getElementById('tab-bar');
const container = document.getElementById('terminal-container');
const overlay  = document.getElementById('overlay');
const overlayMsg = document.getElementById('overlay-msg');
const connStatus = document.getElementById('conn-status');

function showOverlay(msg) {
  overlayMsg.textContent = msg;
  overlay.classList.remove('hidden');
}

function hideOverlay() {
  overlay.classList.add('hidden');
}

function setConnStatus(state /* 'connected' | 'error' | 'idle' */) {
  connStatus.className = 'status-dot ' + state;
}

/* ── Create a new terminal tab ───────────────────────────────────────────── */
async function createTab() {
  showOverlay('Starting terminal…');

  // Determine initial size from container
  const cw = container.clientWidth;
  const ch = container.clientHeight;
  const charW = 9, charH = 17; // approximate — FitAddon will correct
  const cols = Math.max(20, Math.floor((cw - 8) / charW));
  const rows = Math.max(5, Math.floor((ch - 8) / charH));

  let sessionId;
  try {
    const res = await apiFetch('/api/terminal/create', {
      method: 'POST',
      body: JSON.stringify({ cols, rows }),
    });
    if (!res.ok) throw new Error(await res.text());
    const data = await res.json();
    sessionId = data.session_id;
  } catch (e) {
    showOverlay('Failed to create terminal: ' + e.message);
    setConnStatus('error');
    return;
  }

  // DOM
  const tabId = ++tabCounter;
  const pane = document.createElement('div');
  pane.className = 'term-pane';
  pane.dataset.tabId = tabId;
  container.appendChild(pane);

  // Tab button
  const tabEl = document.createElement('div');
  tabEl.className = 'tab';
  tabEl.dataset.tabId = tabId;
  tabEl.innerHTML = `<span class="tab-label">Terminal ${tabId}</span>` +
    `<span class="tab-close" data-tab-id="${tabId}" title="Close">×</span>`;
  tabBar.appendChild(tabEl);

  // xterm
  const term = new Terminal({
    cursorBlink: true,
    cursorStyle: 'block',
    fontSize: 14,
    fontFamily: "'Cascadia Code', 'Fira Code', 'JetBrains Mono', 'Menlo', 'DejaVu Sans Mono', monospace",
    theme: {
      background:       '#000000',
      foreground:       '#d4d4d4',
      cursor:           '#d4d4d4',
      cursorAccent:     '#000000',
      black:            '#1e1e1e',
      red:              '#f44747',
      green:            '#6a9955',
      yellow:           '#d7ba7d',
      blue:             '#569cd6',
      magenta:          '#c586c0',
      cyan:             '#4ec9b0',
      white:            '#d4d4d4',
      brightBlack:      '#808080',
      brightRed:        '#f44747',
      brightGreen:      '#6a9955',
      brightYellow:     '#d7ba7d',
      brightBlue:       '#569cd6',
      brightMagenta:    '#c586c0',
      brightCyan:       '#4ec9b0',
      brightWhite:      '#ffffff',
    },
    allowProposedApi: true,
    scrollback: 5000,
    // Copy on select
    copyOnSelect: false,
    macOptionIsMeta: true,
    rightClickSelectsWord: false,
  });

  const fitAddon = new FitAddon.FitAddon();
  const webLinksAddon = new WebLinksAddon.WebLinksAddon();
  term.loadAddon(fitAddon);
  term.loadAddon(webLinksAddon);
  term.open(pane);

  const tab = {
    id: tabId,
    sessionId,
    term,
    fitAddon,
    pane,
    tabEl,
    offset: 0,
    pollCtrl: null,       // AbortController for current poll
    heartbeatTimer: null,
    closed: false,
  };
  tabs.push(tab);

  // Activate
  activateTab(tabId);
  fitAddon.fit();
  hideOverlay();
  setConnStatus('connected');

  // Send resize after fit
  const dims = fitAddon.proposeDimensions();
  if (dims) sendResize(tab, dims.cols, dims.rows);

  // Handle user input → send to server
  term.onData(data => {
    if (!tab.closed) sendInput(tab, data);
  });

  // Handle binary input (e.g., paste of binary)
  term.onBinary(data => {
    if (!tab.closed) {
      const bytes = new Uint8Array(data.length);
      for (let i = 0; i < data.length; i++) bytes[i] = data.charCodeAt(i) & 0xff;
      sendInputBytes(tab, bytes);
    }
  });

  // Start output polling loop
  startPollLoop(tab);

  // Heartbeat
  tab.heartbeatTimer = setInterval(() => sendHeartbeat(tab), HEARTBEAT_MS);

  // Tab click → activate
  tabEl.addEventListener('click', (e) => {
    if (e.target.classList.contains('tab-close')) {
      closeTab(tabId);
    } else {
      activateTab(tabId);
    }
  });

  return tab;
}

/* ── Tab activation ─────────────────────────────────────────────────────── */
function activateTab(tabId) {
  tabs.forEach(t => {
    const active = t.id === tabId;
    t.pane.classList.toggle('active', active);
    t.tabEl.classList.toggle('active', active);
    if (active) {
      setTimeout(() => {
        t.fitAddon.fit();
        t.term.focus();
      }, 10);
    }
  });
}

function activeTab() {
  return tabs.find(t => t.pane.classList.contains('active')) || tabs[0];
}

/* ── Close a tab ─────────────────────────────────────────────────────────── */
async function closeTab(tabId) {
  const idx = tabs.findIndex(t => t.id === tabId);
  if (idx === -1) return;
  const tab = tabs[idx];

  tab.closed = true;

  if (tab.pollCtrl) tab.pollCtrl.abort();
  clearInterval(tab.heartbeatTimer);

  // Tell server
  try {
    await apiFetch('/api/terminal/close', {
      method: 'POST',
      body: JSON.stringify({ session_id: tab.sessionId }),
    });
  } catch (_) {}

  tab.term.dispose();
  tab.pane.remove();
  tab.tabEl.remove();
  tabs.splice(idx, 1);

  if (tabs.length === 0) {
    showOverlay('All terminals closed. Click "+ New Tab" to start a new session.');
    setConnStatus('idle');
  } else {
    // Activate neighbor
    const newIdx = Math.min(idx, tabs.length - 1);
    activateTab(tabs[newIdx].id);
  }
}

/* ── Send helpers ────────────────────────────────────────────────────────── */
async function sendInput(tab, str) {
  try {
    await apiFetch('/api/terminal/input', {
      method: 'POST',
      body: JSON.stringify({ session_id: tab.sessionId, data: b64encode(str) }),
    });
  } catch (_) {}
}

async function sendInputBytes(tab, bytes) {
  try {
    await apiFetch('/api/terminal/input', {
      method: 'POST',
      body: JSON.stringify({ session_id: tab.sessionId, data: b64encodeBytes(bytes) }),
    });
  } catch (_) {}
}

async function sendResize(tab, cols, rows) {
  try {
    await apiFetch('/api/terminal/resize', {
      method: 'POST',
      body: JSON.stringify({ session_id: tab.sessionId, cols, rows }),
    });
  } catch (_) {}
}

async function sendHeartbeat(tab) {
  if (tab.closed) return;
  try {
    const res = await apiFetch('/api/terminal/heartbeat', {
      method: 'POST',
      body: JSON.stringify({ session_id: tab.sessionId }),
    });
    if (res.status === 404) {
      // Session died server-side
      tab.term.writeln('\r\n\x1b[31m[Session expired. Close this tab.]\x1b[0m');
      tab.closed = true;
      setConnStatus('error');
      if (tab.pollCtrl) tab.pollCtrl.abort();
      clearInterval(tab.heartbeatTimer);
    }
  } catch (_) {}
}

/* ── Long-poll output loop ───────────────────────────────────────────────── */
function startPollLoop(tab) {
  (async function loop() {
    while (!tab.closed) {
      tab.pollCtrl = new AbortController();
      const signal = tab.pollCtrl.signal;

      try {
        const url = `/api/terminal/output?session_id=${encodeURIComponent(tab.sessionId)}&offset=${tab.offset}`;
        const res = await fetch(url, {
          credentials: 'same-origin',
          signal,
        });

        if (res.status === 401) { window.location.replace('/login'); return; }
        if (res.status === 404) {
          tab.term.writeln('\r\n\x1b[31m[Terminal session ended.]\x1b[0m');
          tab.closed = true;
          setConnStatus('error');
          break;
        }

        const data = await res.json();

        if (data.data) {
          const bytes = b64decode(data.data);
          tab.term.write(bytes);
        }

        tab.offset = data.next_offset;

        if (data.closed) {
          tab.term.writeln('\r\n\x1b[33m[Process exited. Close this tab or keep scrolling.]\x1b[0m');
          tab.closed = true;
          clearInterval(tab.heartbeatTimer);
          setConnStatus('error');
          break;
        }

        setConnStatus('connected');

      } catch (err) {
        if (err.name === 'AbortError') break; // tab closed
        // Network error: wait before retry
        setConnStatus('error');
        await sleep(2000);
        setConnStatus('connected');
      }
    }
  })();
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/* ── Resize observer ─────────────────────────────────────────────────────── */
const resizeObserver = new ResizeObserver(() => {
  const tab = activeTab();
  if (!tab || tab.closed) return;
  tab.fitAddon.fit();
  const dims = tab.fitAddon.proposeDimensions();
  if (dims) sendResize(tab, dims.cols, dims.rows);
});
resizeObserver.observe(container);

/* ── Keyboard shortcut: Ctrl+Shift+C → copy selection ───────────────────── */
document.addEventListener('keydown', (e) => {
  // Ctrl+Shift+C or Cmd+C (mac) to copy selected text
  if ((e.ctrlKey && e.shiftKey && e.key === 'C') || (e.metaKey && e.key === 'c')) {
    const tab = activeTab();
    if (!tab) return;
    const sel = tab.term.getSelection();
    if (sel) {
      navigator.clipboard.writeText(sel).catch(() => {});
      e.preventDefault();
    }
  }

  // Ctrl+Shift+V or Cmd+V to paste
  if ((e.ctrlKey && e.shiftKey && e.key === 'V') || (e.metaKey && e.key === 'v')) {
    const tab = activeTab();
    if (!tab) return;
    navigator.clipboard.readText().then(text => {
      if (text) sendInput(tab, text);
    }).catch(() => {});
    e.preventDefault();
  }
});

/* ── Context menu: copy / paste ──────────────────────────────────────────── */
// xterm's built-in right-click context menu is provided by the terminal itself.
// We additionally handle right-click on the terminal container for copy/paste.
container.addEventListener('contextmenu', async (e) => {
  // Let xterm handle its own context menu natively — this ensures selected text
  // is available. We rely on the browser's built-in copy/paste here.
  // On browsers that support it, Ctrl+Shift+C / V is the shortcut (handled above).
});

/* ── New tab button ──────────────────────────────────────────────────────── */
document.getElementById('new-tab-btn').addEventListener('click', () => {
  createTab();
});

/* ── Logout button ───────────────────────────────────────────────────────── */
document.getElementById('logout-btn').addEventListener('click', async () => {
  // Close all sessions gracefully
  for (const tab of [...tabs]) {
    await closeTab(tab.id).catch(() => {});
  }
  window.location.replace('/logout');
});

/* ── Boot: open first terminal ───────────────────────────────────────────── */
(async () => {
  await createTab();
})();
