// Package debug содержит HTTP-обработчик отладочного web-клиента.
package debug

import (
	"net/http"
)

// Handle отдает отладочную HTML-страницу для ручного тестирования API/WS.
func Handle(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write([]byte(pageHTML))
}

const pageHTML = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>my-chat debug client</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 20px; }
    h1 { margin: 0 0 12px; }
    .grid { display: grid; gap: 12px; max-width: 960px; }
    .card { border: 1px solid #ddd; border-radius: 8px; padding: 12px; }
    input, textarea, button, select { width: 100%; padding: 8px; margin-top: 6px; box-sizing: border-box; }
    textarea { min-height: 120px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
    .row { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
    .small { font-size: 12px; color: #666; }
    .ok { color: #0a7f3f; }
    .err { color: #b00020; }
  </style>
</head>
<body>
  <h1>my-chat debug client</h1>
  <p class="small">
    Страница для отладки backend без мобильного клиента.
    Поддерживает ручные HTTP-вызовы и WebSocket-соединение.
  </p>

  <div class="grid">
    <section class="card">
      <strong>1) Базовые настройки</strong>
      <label>Base URL HTTP
        <input id="baseUrl" value="http://localhost:8080" />
      </label>
      <label>Access token (опционально)
        <input id="token" placeholder="Bearer token без префикса" />
      </label>
      <label>Текущий status
        <input id="status" readonly />
      </label>
      <button id="checkHealth">Проверить /health</button>
    </section>

    <section class="card">
      <strong>2) HTTP запрос</strong>
      <div class="row">
        <label>Method
          <select id="method">
            <option>GET</option>
            <option>POST</option>
            <option>PUT</option>
            <option>DELETE</option>
          </select>
        </label>
        <label>Path
          <input id="path" value="/health" />
        </label>
      </div>
      <label>JSON body
        <textarea id="body" placeholder='{"example":"value"}'></textarea>
      </label>
      <button id="sendHttp">Отправить HTTP</button>
    </section>

    <section class="card">
      <strong>3) WebSocket</strong>
      <label>WebSocket URL
        <input id="wsUrl" value="ws://localhost:8080/ws/connect" />
      </label>
      <div class="row">
        <button id="wsConnect">Подключить WS</button>
        <button id="wsClose">Отключить WS</button>
      </div>
      <label>Сообщение в WS
        <textarea id="wsOut" placeholder='{"type":"ping"}'></textarea>
      </label>
      <button id="wsSend">Отправить в WS</button>
    </section>

    <section class="card">
      <strong>Лог</strong>
      <textarea id="log" readonly></textarea>
    </section>
  </div>

  <script>
    const $ = (id) => document.getElementById(id);
    const logEl = $("log");
    const statusEl = $("status");
    let socket = null;

    function log(kind, msg) {
      const ts = new Date().toISOString();
      logEl.value += "[" + ts + "] " + kind + " " + msg + "\n";
      logEl.scrollTop = logEl.scrollHeight;
    }

    function setStatus(text, isError) {
      statusEl.value = text;
      statusEl.className = isError ? "err" : "ok";
    }

    function authHeaders() {
      const token = $("token").value.trim();
      if (!token) return {};
      return { Authorization: "Bearer " + token };
    }

    $("checkHealth").onclick = async () => {
      const base = $("baseUrl").value.trim().replace(/\/+$/, "");
      try {
        const res = await fetch(base + "/health", { headers: authHeaders() });
        const text = await res.text();
        setStatus("health: " + res.status, !res.ok);
        log("HTTP", "GET /health -> " + res.status + " " + text);
      } catch (err) {
        setStatus("health error", true);
        log("ERR", String(err));
      }
    };

    $("sendHttp").onclick = async () => {
      const base = $("baseUrl").value.trim().replace(/\/+$/, "");
      const method = $("method").value;
      const path = $("path").value.trim();
      const bodyRaw = $("body").value.trim();
      const headers = { "Content-Type": "application/json", ...authHeaders() };
      const opts = { method, headers };

      if (bodyRaw && method !== "GET" && method !== "DELETE") {
        opts.body = bodyRaw;
      }

      try {
        const res = await fetch(base + path, opts);
        const text = await res.text();
        setStatus("http: " + res.status, !res.ok);
        log("HTTP", method + " " + path + " -> " + res.status + " " + text);
      } catch (err) {
        setStatus("http error", true);
        log("ERR", String(err));
      }
    };

    $("wsConnect").onclick = () => {
      const url = $("wsUrl").value.trim();
      if (!url) {
        log("ERR", "empty ws url");
        return;
      }

      if (socket && socket.readyState === WebSocket.OPEN) {
        log("WS", "already connected");
        return;
      }

      socket = new WebSocket(url);
      socket.onopen = () => {
        setStatus("ws: connected", false);
        log("WS", "connected");
      };
      socket.onclose = (ev) => {
        setStatus("ws: closed", false);
        log("WS", "closed code=" + ev.code);
      };
      socket.onerror = (ev) => {
        setStatus("ws: error", true);
        log("ERR", "ws error " + JSON.stringify(ev));
      };
      socket.onmessage = (ev) => {
        log("WS<", ev.data);
      };
    };

    $("wsClose").onclick = () => {
      if (!socket) {
        log("WS", "socket is not initialized");
        return;
      }
      socket.close();
    };

    $("wsSend").onclick = () => {
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        log("ERR", "ws not connected");
        return;
      }
      const payload = $("wsOut").value;
      socket.send(payload);
      log("WS>", payload);
    };
  </script>
</body>
</html>
`
