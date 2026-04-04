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
      <label>Base URL HTTP (main-service)
        <input id="baseUrl" value="http://localhost:8080" />
      </label>
      <label>Auth URL (auth-proxy)
        <input id="authUrl" value="http://localhost:33081" />
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
      <p class="small">Токен из раздела 1 добавится автоматически как ?token= при подключении.</p>
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
      <strong>4) Шорткаты</strong>
      <div class="row">
        <div>
          <strong class="small">Login</strong>
          <label>user_id
            <input id="scUserId" placeholder="11111111-1111-1111-1111-111111111111" />
          </label>
          <button id="scLogin">Login (сохранить токен)</button>
        </div>
        <div>
          <strong class="small">Unread count</strong>
          <button id="scUnread" style="margin-top:28px">GET /me/unread-count</button>
        </div>
      </div>
      <div class="row" style="margin-top:8px">
        <div>
          <strong class="small">Send message</strong>
          <label>dialog_id
            <input id="scDialogId" placeholder="bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" />
          </label>
          <label>body
            <input id="scBody" placeholder="hello" />
          </label>
          <button id="scSend">POST send message</button>
        </div>
        <div>
          <strong class="small">Mark read</strong>
          <label>message_id
            <input id="scMessageId" placeholder="aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" />
          </label>
          <button id="scRead" style="margin-top:28px">POST mark read</button>
        </div>
      </div>
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
      let url = $("wsUrl").value.trim();
      if (!url) {
        log("ERR", "empty ws url");
        return;
      }

      const token = $("token").value.trim();
      if (token) {
        url += (url.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(token);
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

    function scBase() {
      return $("baseUrl").value.trim().replace(/\/+$/, "");
    }

    function scHeaders() {
      const token = $("token").value.trim();
      const h = { "Content-Type": "application/json" };
      if (token) h["Authorization"] = "Bearer " + token;
      return h;
    }

    $("scLogin").onclick = async () => {
      const userID = $("scUserId").value.trim();
      if (!userID) { log("ERR", "user_id is empty"); return; }
      const authBase = $("authUrl").value.trim().replace(/\/+$/, "");
      try {
        const res = await fetch(authBase + "/api/v1/auth/login", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ user_id: userID }),
        });
        const data = await res.json();
        setStatus("login: " + res.status, !res.ok);
        log("AUTH", "login " + userID + " -> " + res.status + " " + JSON.stringify(data));
        if (data.access_token) {
          $("token").value = data.access_token;
          log("AUTH", "token saved");
        }
      } catch (err) {
        setStatus("login error", true);
        log("ERR", String(err));
      }
    };

    $("scSend").onclick = async () => {
      const dialogID = $("scDialogId").value.trim();
      const body = $("scBody").value.trim();
      if (!dialogID) { log("ERR", "dialog_id is empty"); return; }
      if (!body) { log("ERR", "body is empty"); return; }
      try {
        const res = await fetch(scBase() + "/api/v1/dialogs/" + dialogID + "/messages", {
          method: "POST",
          headers: scHeaders(),
          body: JSON.stringify({ body }),
        });
        const data = await res.json();
        setStatus("send: " + res.status, !res.ok);
        log("SEND", "-> " + res.status + " " + JSON.stringify(data));
        if (data.message && data.message.id) {
          $("scMessageId").value = data.message.id;
          log("SEND", "message_id saved: " + data.message.id);
        }
      } catch (err) {
        setStatus("send error", true);
        log("ERR", String(err));
      }
    };

    $("scRead").onclick = async () => {
      const msgID = $("scMessageId").value.trim();
      if (!msgID) { log("ERR", "message_id is empty"); return; }
      try {
        const res = await fetch(scBase() + "/api/v1/messages/" + msgID + "/read", {
          method: "POST",
          headers: scHeaders(),
        });
        setStatus("read: " + res.status, !res.ok);
        log("READ", msgID + " -> " + res.status);
      } catch (err) {
        setStatus("read error", true);
        log("ERR", String(err));
      }
    };

    $("scUnread").onclick = async () => {
      try {
        const res = await fetch(scBase() + "/api/v1/me/unread-count", {
          headers: scHeaders(),
        });
        const data = await res.json();
        setStatus("unread: " + res.status, !res.ok);
        log("UNREAD", "-> " + res.status + " " + JSON.stringify(data));
      } catch (err) {
        setStatus("unread error", true);
        log("ERR", String(err));
      }
    };
  </script>
</body>
</html>
`
