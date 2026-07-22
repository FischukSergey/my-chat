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
    .warn { color: #b05f00; }
    .badge { display: inline-block; font-size: 10px; padding: 2px 6px; border-radius: 4px;
             margin-left: 4px; background: #e8f4e8; color: #0a7f3f; }
    .badge-warn { background: #fff3e0; color: #b05f00; }
  </style>
</head>
<body>
  <h1>my-chat debug client</h1>
  <p class="small">
    Страница для отладки backend без мобильного клиента.
    Поддерживает ручные HTTP-вызовы и WebSocket-соединение.
    Токены сохраняются в <code>localStorage</code>.
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
      <div class="row">
        <label>Access token
          <input id="token" placeholder="Bearer token без префикса" />
        </label>
        <label>Session ID <span class="badge" id="sessionBadge">—</span>
          <input id="sessionId" readonly placeholder="из последнего login/refresh" />
        </label>
      </div>
      <label>Refresh token
        <input id="refreshToken" readonly placeholder="сохраняется при login/refresh" style="font-size:11px" />
      </label>
      <label>Текущий status
        <input id="status" readonly />
      </label>
      <div class="row">
        <button id="checkHealth">Проверить /health</button>
        <button id="clearTokens" style="margin-top:6px">Очистить токены</button>
      </div>
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
      <div class="row">
        <button id="sendHttp">Отправить HTTP</button>
        <button id="sendHttpAutoRefresh">Отправить + auto-refresh при 401</button>
      </div>
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
      <strong>4) Шорткаты — Auth</strong>
      <div class="row">
        <div>
          <strong class="small">Login</strong>
          <label>user_id
            <input id="scUserId" placeholder="11111111-1111-1111-1111-111111111111" />
          </label>
          <button id="scLogin">Login (сохранить токен)</button>
        </div>
        <div>
          <strong class="small">Refresh <span class="badge" id="refreshBadge">нет токена</span></strong>
          <p class="small" style="margin:6px 0">Ротирует refresh-токен, выдаёт новую пару.</p>
          <button id="scRefresh" style="margin-top:6px">Refresh</button>
        </div>
      </div>
      <div class="row" style="margin-top:8px">
        <div>
          <strong class="small">Logout</strong>
          <p class="small" style="margin:6px 0">Отзывает текущую сессию на сервере.</p>
          <button id="scLogout" style="margin-top:6px">Logout</button>
        </div>
        <div>
          <strong class="small">Reuse test <span class="badge badge-warn" id="reuseBadge">нет старого токена</span></strong>
          <p class="small" style="margin:6px 0">Повторно отправляет предыдущий
            refresh-токен. Ожидается <code>session_compromised</code>.</p>
          <button id="scReuseTest" style="margin-top:6px">Reuse test</button>
        </div>
      </div>
    </section>

    <section class="card">
      <strong>5) Шорткаты — Чат</strong>
      <div class="row">
        <div>
          <strong class="small">Unread count</strong>
          <button id="scUnread" style="margin-top:28px">GET /me/unread-count</button>
        </div>
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
      </div>
      <div class="row" style="margin-top:8px">
        <div>
          <strong class="small">Mark read</strong>
          <label>message_id
            <input id="scMessageId" placeholder="aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" />
          </label>
          <button id="scRead" style="margin-top:28px">POST mark read</button>
        </div>
        <div></div>
      </div>
    </section>

    <section class="card">
      <strong>6) Шорткаты — Устройства</strong>
      <div class="row">
        <div>
          <strong class="small">Device register</strong>
          <label>platform
            <select id="scDevicePlatform">
              <option value="ios">ios</option>
              <option value="android">android</option>
              <option value="web">web</option>
            </select>
          </label>
          <label>push_token
            <input id="scDeviceToken" placeholder="fake-push-token-local" />
          </label>
          <button id="scDeviceRegister">POST devices/register</button>
        </div>
        <div>
          <strong class="small">Device unregister</strong>
          <p class="small" style="margin:6px 0">Использует platform и push_token из блока Register.</p>
          <button id="scDeviceUnregister" style="margin-top:6px">POST devices/unregister</button>
        </div>
      </div>
    </section>

    <section class="card">
      <strong>Лог</strong>
      <button id="clearLog" style="width:auto;padding:4px 12px;margin-bottom:4px">Очистить лог</button>
      <textarea id="log" readonly></textarea>
    </section>
  </div>

  <script>
    const $ = (id) => document.getElementById(id);
    const logEl = $("log");
    const statusEl = $("status");
    let socket = null;
    let prevRefreshToken = "";   // для reuse test

    // --- localStorage ---

    const LS_ACCESS  = "my_chat_access_token";
    const LS_REFRESH = "my_chat_refresh_token";
    const LS_SESSION = "my_chat_session_id";

    function saveTokens(access, refresh, sessionId) {
      if (access)    { localStorage.setItem(LS_ACCESS,  access);    $("token").value      = access; }
      if (refresh)   { localStorage.setItem(LS_REFRESH, refresh);   $("refreshToken").value = refresh; }
      if (sessionId) { localStorage.setItem(LS_SESSION, sessionId); $("sessionId").value  = sessionId; }
      updateBadges();
    }

    function clearTokens() {
      localStorage.removeItem(LS_ACCESS);
      localStorage.removeItem(LS_REFRESH);
      localStorage.removeItem(LS_SESSION);
      $("token").value       = "";
      $("refreshToken").value = "";
      $("sessionId").value   = "";
      prevRefreshToken       = "";
      updateBadges();
    }

    function loadTokens() {
      $("token").value        = localStorage.getItem(LS_ACCESS)  || "";
      $("refreshToken").value = localStorage.getItem(LS_REFRESH) || "";
      $("sessionId").value    = localStorage.getItem(LS_SESSION) || "";
      updateBadges();
    }

    function updateBadges() {
      const hasRefresh = !!$("refreshToken").value;
      const rf = $("refreshBadge");
      rf.textContent = hasRefresh ? "есть токен" : "нет токена";
      rf.className   = "badge" + (hasRefresh ? "" : " badge-warn");

      const rb = $("reuseBadge");
      rb.textContent = prevRefreshToken ? "есть старый токен" : "нет старого токена";
      rb.className   = "badge" + (prevRefreshToken ? "" : " badge-warn");

      const sid = $("sessionId").value;
      const sb  = $("sessionBadge");
      sb.textContent = sid ? sid.slice(0, 8) + "…" : "—";
    }

    // --- Log ---

    function log(kind, msg) {
      const ts = new Date().toISOString();
      logEl.value += "[" + ts + "] " + kind + " " + msg + "\n";
      logEl.scrollTop = logEl.scrollHeight;
    }

    function setStatus(text, isError) {
      statusEl.value = text;
      statusEl.className = isError ? "err" : "ok";
    }

    // --- Headers ---

    function authHeaders() {
      const token = $("token").value.trim();
      if (!token) return {};
      return { Authorization: "Bearer " + token };
    }

    function scHeaders() {
      const token = $("token").value.trim();
      const h = { "Content-Type": "application/json" };
      if (token) h["Authorization"] = "Bearer " + token;
      return h;
    }

    function authBase() {
      return $("authUrl").value.trim().replace(/\/+$/, "");
    }

    function scBase() {
      return $("baseUrl").value.trim().replace(/\/+$/, "");
    }

    // --- Auth core ---

    async function performRefresh(refreshToken) {
      const res = await fetch(authBase() + "/api/v1/auth/refresh", {
        method:  "POST",
        headers: { "Content-Type": "application/json" },
        body:    JSON.stringify({ refresh_token: refreshToken }),
      });
      const data = await res.json();
      return { res, data };
    }

    // --- Buttons ---

    $("clearTokens").onclick = () => {
      clearTokens();
      log("AUTH", "токены очищены");
    };

    $("clearLog").onclick = () => { logEl.value = ""; };

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

    async function doHttpRequest(autoRefresh) {
      const base    = $("baseUrl").value.trim().replace(/\/+$/, "");
      const method  = $("method").value;
      const path    = $("path").value.trim();
      const bodyRaw = $("body").value.trim();
      const headers = { "Content-Type": "application/json", ...authHeaders() };
      const opts    = { method, headers };

      if (bodyRaw && method !== "GET" && method !== "DELETE") opts.body = bodyRaw;

      try {
        let res = await fetch(base + path, opts);

        if (res.status === 401 && autoRefresh) {
          const rt = $("refreshToken").value.trim();
          if (rt) {
            log("AUTH", "401 получен — пробуем auto-refresh...");
            const { res: rRes, data: rData } = await performRefresh(rt);
            log("AUTH", "refresh -> " + rRes.status + " " + JSON.stringify(rData));
            if (rRes.ok && rData.access_token) {
              prevRefreshToken = rt;
              saveTokens(rData.access_token, rData.refresh_token, rData.session_id);
              log("AUTH", "auto-refresh успешен, повторяем запрос");
              opts.headers["Authorization"] = "Bearer " + rData.access_token;
              res = await fetch(base + path, opts);
            }
          }
        }

        const text = await res.text();
        setStatus("http: " + res.status, !res.ok);
        log("HTTP", method + " " + path + " -> " + res.status + " " + text);
      } catch (err) {
        setStatus("http error", true);
        log("ERR", String(err));
      }
    }

    $("sendHttp").onclick = () => doHttpRequest(false);
    $("sendHttpAutoRefresh").onclick = () => doHttpRequest(true);

    // --- WebSocket ---

    $("wsConnect").onclick = () => {
      let url = $("wsUrl").value.trim();
      if (!url) { log("ERR", "empty ws url"); return; }

      const token = $("token").value.trim();
      if (token) url += (url.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(token);

      if (socket && socket.readyState === WebSocket.OPEN) {
        log("WS", "already connected");
        return;
      }

      socket = new WebSocket(url);
      socket.onopen    = () => { setStatus("ws: connected", false); log("WS", "connected"); };
      socket.onclose   = (ev) => { setStatus("ws: closed", false); log("WS", "closed code=" + ev.code); };
      socket.onerror   = (ev) => { setStatus("ws: error", true); log("ERR", "ws error " + JSON.stringify(ev)); };
      socket.onmessage = (ev) => { log("WS<", ev.data); };
    };

    $("wsClose").onclick = () => {
      if (!socket) { log("WS", "socket is not initialized"); return; }
      socket.close();
    };

    $("wsSend").onclick = () => {
      if (!socket || socket.readyState !== WebSocket.OPEN) { log("ERR", "ws not connected"); return; }
      const payload = $("wsOut").value;
      socket.send(payload);
      log("WS>", payload);
    };

    // --- Auth shortcuts ---

    $("scLogin").onclick = async () => {
      const userID = $("scUserId").value.trim();
      if (!userID) { log("ERR", "user_id is empty"); return; }
      try {
        const res  = await fetch(authBase() + "/api/v1/auth/login", {
          method:  "POST",
          headers: { "Content-Type": "application/json" },
          body:    JSON.stringify({ user_id: userID }),
        });
        const data = await res.json();
        setStatus("login: " + res.status, !res.ok);
        log("AUTH", "login " + userID + " -> " + res.status + " " + JSON.stringify(data));
        if (res.ok && data.access_token) {
          prevRefreshToken = "";
          saveTokens(data.access_token, data.refresh_token, data.session_id);
          log("AUTH", "access + refresh token сохранены (localStorage)");
        }
      } catch (err) {
        setStatus("login error", true);
        log("ERR", String(err));
      }
    };

    $("scRefresh").onclick = async () => {
      const rt = $("refreshToken").value.trim();
      if (!rt) { log("ERR", "refresh_token пуст — сначала выполните Login"); return; }
      try {
        prevRefreshToken = rt;
        const { res, data } = await performRefresh(rt);
        setStatus("refresh: " + res.status, !res.ok);
        log("AUTH", "refresh -> " + res.status + " " + JSON.stringify(data));
        if (res.ok && data.access_token) {
          saveTokens(data.access_token, data.refresh_token, data.session_id);
          log("AUTH", "новые токены сохранены; старый refresh готов для reuse test");
          updateBadges();
        }
      } catch (err) {
        setStatus("refresh error", true);
        log("ERR", String(err));
      }
    };

    $("scLogout").onclick = async () => {
      const rt = $("refreshToken").value.trim();
      if (!rt) { log("ERR", "refresh_token пуст — нет активной сессии"); return; }
      try {
        const res = await fetch(authBase() + "/api/v1/auth/logout", {
          method:  "POST",
          headers: { "Content-Type": "application/json" },
          body:    JSON.stringify({ refresh_token: rt }),
        });
        setStatus("logout: " + res.status, !res.ok);
        if (res.status === 204) {
          log("AUTH", "logout -> 204 сессия отозвана");
          clearTokens();
          log("AUTH", "токены очищены из localStorage");
        } else {
          const data = await res.json();
          log("AUTH", "logout -> " + res.status + " " + JSON.stringify(data));
        }
      } catch (err) {
        setStatus("logout error", true);
        log("ERR", String(err));
      }
    };

    $("scReuseTest").onclick = async () => {
      if (!prevRefreshToken) {
        log("ERR", "нет сохранённого старого refresh-токена — сначала выполните Refresh");
        return;
      }
      log("AUTH", "reuse test: повторная отправка старого refresh-токена...");
      try {
        const { res, data } = await performRefresh(prevRefreshToken);
        setStatus("reuse: " + res.status, !res.ok);
        log("AUTH", "reuse test -> " + res.status + " " + JSON.stringify(data));
        if (res.status === 401 && data.error && data.error.code === "session_compromised") {
          log("AUTH", "✓ session_compromised — reuse detection сработал, family отозвана");
          clearTokens();
          log("AUTH", "токены очищены (family revoked)");
        }
      } catch (err) {
        setStatus("reuse error", true);
        log("ERR", String(err));
      }
    };

    // --- Chat shortcuts ---

    $("scUnread").onclick = async () => {
      try {
        const res  = await fetch(scBase() + "/api/v1/me/unread-count", { headers: scHeaders() });
        const data = await res.json();
        setStatus("unread: " + res.status, !res.ok);
        log("UNREAD", "-> " + res.status + " " + JSON.stringify(data));
      } catch (err) {
        setStatus("unread error", true);
        log("ERR", String(err));
      }
    };

    $("scSend").onclick = async () => {
      const dialogID = $("scDialogId").value.trim();
      const body     = $("scBody").value.trim();
      if (!dialogID) { log("ERR", "dialog_id is empty"); return; }
      if (!body)     { log("ERR", "body is empty"); return; }
      try {
        const res  = await fetch(scBase() + "/api/v1/dialogs/" + dialogID + "/messages", {
          method:  "POST",
          headers: scHeaders(),
          body:    JSON.stringify({ body }),
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
          method:  "POST",
          headers: scHeaders(),
        });
        setStatus("read: " + res.status, !res.ok);
        log("READ", msgID + " -> " + res.status);
      } catch (err) {
        setStatus("read error", true);
        log("ERR", String(err));
      }
    };

    // --- Device shortcuts ---

    $("scDeviceRegister").onclick = async () => {
      const platform  = $("scDevicePlatform").value;
      const pushToken = $("scDeviceToken").value.trim();
      if (!pushToken) { log("ERR", "push_token is empty"); return; }
      try {
        const res  = await fetch(scBase() + "/api/v1/devices/register", {
          method:  "POST",
          headers: scHeaders(),
          body:    JSON.stringify({ platform, push_token: pushToken }),
        });
        const data = await res.json();
        setStatus("device register: " + res.status, !res.ok);
        log("DEVICE", "register -> " + res.status + " " + JSON.stringify(data));
      } catch (err) {
        setStatus("device register error", true);
        log("ERR", String(err));
      }
    };

    $("scDeviceUnregister").onclick = async () => {
      const platform  = $("scDevicePlatform").value;
      const pushToken = $("scDeviceToken").value.trim();
      if (!pushToken) { log("ERR", "push_token is empty"); return; }
      try {
        const res = await fetch(scBase() + "/api/v1/devices/unregister", {
          method:  "POST",
          headers: scHeaders(),
          body:    JSON.stringify({ platform, push_token: pushToken }),
        });
        setStatus("device unregister: " + res.status, !res.ok);
        log("DEVICE", "unregister -> " + res.status + (res.status === 204 ? " (ok)" : ""));
      } catch (err) {
        setStatus("device unregister error", true);
        log("ERR", String(err));
      }
    };

    // --- Init ---
    loadTokens();
  </script>
</body>
</html>
`
