/**
 * main.ts — точка входа. Управляет состояниями экранов:
 *
 *   [login]  ← нет refresh token / session_compromised / logout
 *      ↓ после login
 *   [unlock] ← есть refresh token (cold start)
 *      ↓ биометрия + refresh
 *   [home]   ← есть актуальный access token
 *      ↓ showChat(dialogId)
 *   [chat]   ← открытый диалог
 */

import {
  BiometryErrorType,
  clearAllTokens,
  getAccessToken,
  getRefreshTokenRaw,
  getSavedUserId,
  hasRefreshToken,
  isBiometricAvailable,
  saveTokens,
  getRefreshToken,
} from "./auth";
import {
  apiLogin,
  apiLogout,
  apiRefresh,
  getMessages,
  getUnreadCount,
  markRead,
  sendMessage,
  SessionCompromisedError,
  SessionExpiredError,
  SessionRevokedError,
  type Message,
} from "./api";

// --- DOM helpers ---

function el<T extends HTMLElement>(id: string): T {
  const e = document.getElementById(id);
  if (!e) throw new Error(`Element #${id} not found`);
  return e as T;
}

type ScreenName = "login" | "unlock" | "home" | "chat";

function showScreen(name: ScreenName): void {
  for (const s of ["login", "unlock", "home", "chat"] as const) {
    document.getElementById(s)?.classList.toggle("active", s === name);
  }
  // Скрываем статус-бар и лог-панель в режиме чата
  const statusBar = document.getElementById("status-bar");
  const logDetails = document.getElementById("log-details");
  if (statusBar) statusBar.style.display = name === "chat" ? "none" : "";
  if (logDetails) (logDetails as HTMLElement).style.display = name === "chat" ? "none" : "";
}

function setStatus(msg: string, isError = false): void {
  const s = el("status-bar");
  s.textContent = msg;
  s.className = isError ? "error" : "";
}

function log(msg: string): void {
  const l = el<HTMLTextAreaElement>("log");
  l.value += `[${new Date().toISOString()}] ${msg}\n`;
  l.scrollTop = l.scrollHeight;
}

// --- WebSocket ---

let ws: WebSocket | null = null;
let wsReconnectTimer: ReturnType<typeof setTimeout> | null = null;

function buildWsUrl(token: string): string {
  // VITE_WS_URL задаётся в .env для dev-режима (ws://localhost:8080).
  // В продакшне и в Capacitor WS идёт через тот же хост, что и API.
  const wsBase = import.meta.env.VITE_WS_URL as string | undefined;
  if (wsBase) {
    return `${wsBase}/ws/connect?token=${encodeURIComponent(token)}`;
  }

  const apiBase = import.meta.env.VITE_API_URL as string | undefined;
  const base = apiBase || `${window.location.protocol}//${window.location.host}`;
  const wsUrl = base.replace(/^https:/, "wss:").replace(/^http:/, "ws:");
  return `${wsUrl}/ws/connect?token=${encodeURIComponent(token)}`;
}

function connectWS(token: string): void {
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;

  ws = new WebSocket(buildWsUrl(token));

  ws.addEventListener("open", () => {
    log("WS: подключено");
    if (wsReconnectTimer !== null) {
      clearTimeout(wsReconnectTimer);
      wsReconnectTimer = null;
    }
  });

  ws.addEventListener("message", (ev) => {
    if (typeof ev.data === "string") handleWSMessage(ev.data);
  });

  ws.addEventListener("close", () => {
    log("WS: разрыв, переподключение через 3 сек...");
    wsReconnectTimer = setTimeout(() => void doReconnectWS(), 3000);
  });

  ws.addEventListener("error", () => {
    log("WS: ошибка соединения");
  });
}

async function doReconnectWS(): Promise<void> {
  const token = await getAccessToken();
  if (token) connectWS(token);
}

function disconnectWS(): void {
  if (wsReconnectTimer !== null) {
    clearTimeout(wsReconnectTimer);
    wsReconnectTimer = null;
  }
  if (ws) {
    ws.close();
    ws = null;
  }
}

// --- WS event types (Hub envelope: {event, data, ts}) ---

interface HubEnvelope {
  event: string;
  data: unknown;
  ts: string;
}

interface WSDataMessageNew {
  message_id: string;
  dialog_id: string;
  sender_id: string;
  body: string;
  created_at: string;
  expires_at: string | null;
}
interface WSDataTTLStarted {
  message_id: string;
  dialog_id: string;
  expires_at: string;
}
interface WSDataMessageDeleted {
  message_id: string;
  dialog_id: string;
}
interface WSDataBadgeUpdated {
  unread_count: number;
}

function handleWSMessage(raw: string): void {
  let env: HubEnvelope;
  try {
    env = JSON.parse(raw) as HubEnvelope;
  } catch {
    return;
  }

  log(`WS ← ${env.event}`);

  switch (env.event) {
    case "message_new": {
      const d = env.data as WSDataMessageNew;
      if (currentDialogId && d.dialog_id === currentDialogId) {
        const msg: Message = {
          id: d.message_id,
          dialog_id: d.dialog_id,
          sender_id: d.sender_id,
          body: d.body,
          created_at: d.created_at,
          expires_at: d.expires_at,
        };
        appendBubble(msg);
        scrollToBottom();
        if (msg.sender_id !== currentUserId) {
          tryMarkRead(msg.id);
        }
      }
      break;
    }
    case "message_ttl_started": {
      const d = env.data as WSDataTTLStarted;
      const bubbleEl = document.querySelector<HTMLElement>(`[data-msg-id="${d.message_id}"]`);
      if (bubbleEl) startTTLTimer(d.message_id, d.expires_at, bubbleEl);
      break;
    }
    case "message_deleted": {
      const d = env.data as WSDataMessageDeleted;
      stopTTLTimer(d.message_id);
      const bubbleEl = document.querySelector<HTMLElement>(`[data-msg-id="${d.message_id}"]`);
      if (bubbleEl) bubbleEl.style.display = "none";
      break;
    }
    case "badge_updated": {
      const d = env.data as WSDataBadgeUpdated;
      const countEl = document.getElementById("unread-count");
      if (countEl) countEl.textContent = String(d.unread_count);
      break;
    }
    default:
      break;
  }
}

// --- TTL timers ---

const ttlTimers = new Map<string, ReturnType<typeof setInterval>>();

/**
 * Запускает визуальный обратный отсчёт внутри bubble.
 * Не скрывает bubble самостоятельно — скрытие происходит только по событию message_deleted
 * от сервера (server-driven deletion).
 */
function startTTLTimer(messageId: string, expiresAt: string, bubbleEl: HTMLElement): void {
  stopTTLTimer(messageId);

  const timerEl = bubbleEl.querySelector<HTMLElement>(".ttl-timer");
  if (!timerEl) return;

  const update = (): void => {
    const remaining = Math.max(0, new Date(expiresAt).getTime() - Date.now());
    const totalSecs = Math.ceil(remaining / 1000);
    const mm = String(Math.floor(totalSecs / 60)).padStart(2, "0");
    const ss = String(totalSecs % 60).padStart(2, "0");
    timerEl.textContent = `⏱ ${mm}:${ss}`;
    timerEl.style.display = "";

    if (remaining <= 0) {
      stopTTLTimer(messageId);
      // Ждём message_deleted от сервера — не скрываем здесь.
    }
  };

  update();
  const id = setInterval(update, 1000);
  ttlTimers.set(messageId, id);
}

function stopTTLTimer(messageId: string): void {
  const timerId = ttlTimers.get(messageId);
  if (timerId !== undefined) {
    clearInterval(timerId);
    ttlTimers.delete(messageId);
  }
}

function stopAllTTLTimers(): void {
  for (const msgId of ttlTimers.keys()) {
    stopTTLTimer(msgId);
  }
}

// --- Page Visibility / markRead ---
//
// markRead вызывается только когда диалог видим (document.hidden === false).
// Если вкладка скрыта — ID складываются в pendingMarkRead и отправляются
// при возврате фокуса через visibilitychange.

const pendingMarkRead = new Set<string>();

function tryMarkRead(messageId: string): void {
  if (!document.hidden) {
    void markRead(messageId).catch(() => undefined);
  } else {
    pendingMarkRead.add(messageId);
  }
}

function flushPendingMarkRead(): void {
  if (pendingMarkRead.size === 0) return;
  const ids = [...pendingMarkRead];
  pendingMarkRead.clear();
  for (const id of ids) {
    void markRead(id).catch(() => undefined);
  }
}

// --- Chat screen ---

let currentUserId = "";
let currentDialogId = "";

function escHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function appendBubble(msg: Message): HTMLElement {
  const listEl = el("messages-list");
  const isOutgoing = msg.sender_id === currentUserId;
  const time = new Date(msg.created_at).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
  });

  const bubble = document.createElement("div");
  bubble.className = `bubble ${isOutgoing ? "outgoing" : "incoming"}`;
  bubble.dataset.msgId = msg.id;
  bubble.innerHTML = `
    <div class="bubble-body">${escHtml(msg.body)}</div>
    <div class="bubble-time">${time}</div>
    <div class="ttl-timer" style="display:none"></div>
  `;

  listEl.appendChild(bubble);
  return bubble;
}

function scrollToBottom(): void {
  const listEl = document.getElementById("messages-list");
  if (listEl) listEl.scrollTop = listEl.scrollHeight;
}

async function showChat(dialogId: string): Promise<void> {
  currentDialogId = dialogId;

  const titleEl = document.getElementById("chat-title");
  if (titleEl) titleEl.textContent = `${dialogId.slice(0, 8)}…`;

  stopAllTTLTimers();
  el("messages-list").innerHTML = "";

  showScreen("chat");
  await loadChatHistory();
}

async function loadChatHistory(): Promise<void> {
  try {
    const messages = await getMessages(currentDialogId);
    messages.reverse(); // API возвращает DESC, рендерим старые→сверху, новые→снизу
    const now = Date.now();
    for (const msg of messages) {
      // Сообщение уже истекло по TTL — не рендерим.
      // message_deleted от сервера либо уже пришёл, либо придёт в течение poll-интервала.
      if (msg.expires_at && new Date(msg.expires_at).getTime() <= now) {
        continue;
      }

      const bubbleEl = appendBubble(msg);

      if (msg.expires_at) {
        // TTL уже запущен — показываем обратный отсчёт (сообщение ещё не истекло)
        startTTLTimer(msg.id, msg.expires_at, bubbleEl);
      } else if (msg.sender_id !== currentUserId) {
        // Входящее непрочитанное — помечаем прочитанным если вкладка видима
        tryMarkRead(msg.id);
      }
    }
    scrollToBottom();
  } catch (err) {
    log(`ERR loadChatHistory: ${String(err)}`);
  }
}

async function handleSendMessage(): Promise<void> {
  const input = el<HTMLInputElement>("msg-input");
  const body = input.value.trim();
  if (!body || !currentDialogId) return;

  input.value = "";
  try {
    const msg = await sendMessage(currentDialogId, body);
    appendBubble(msg);
    scrollToBottom();
    log(`sendMessage OK: ${msg.id}`);
  } catch (err) {
    input.value = body;
    log(`ERR sendMessage: ${String(err)}`);
  }
}

function handleBackFromChat(): void {
  stopAllTTLTimers();
  pendingMarkRead.clear();
  currentDialogId = "";
  void loadHome();
}

// --- Экран: Home ---

async function loadHome(): Promise<void> {
  showScreen("home");
  setStatus("Загрузка...");

  currentUserId = (await getSavedUserId()) ?? "";

  // Подключаем WS если ещё не подключены
  const token = await getAccessToken();
  if (token) connectWS(token);

  try {
    const count = await getUnreadCount();
    el("unread-count").textContent = String(count);
    setStatus("Загружено");
    log(`GET /me/unread-count → ${count}`);
  } catch (err) {
    if (
      err instanceof SessionCompromisedError ||
      err instanceof SessionExpiredError ||
      err instanceof SessionRevokedError
    ) {
      log(`Auth error: ${String(err)}`);
      setStatus(err.message, true);
      await clearAllTokens();
      showLoginScreen();
      return;
    }
    setStatus(String(err), true);
    log(`ERR: ${String(err)}`);
  }
}

// --- Экран: Unlock (cold start) ---

/**
 * Путь разблокировки в среде без биометрии (браузер, web dev).
 * Refresh-токен читается напрямую без биометрической проверки.
 */
async function startUnlockNoBiometric(): Promise<void> {
  showScreen("unlock");
  setStatus("Обновление сессии...");
  log("no-biometric: читаем refresh token напрямую...");
  try {
    const rt = await getRefreshTokenRaw();
    if (!rt) {
      log("refresh token не найден, переход на Login");
      showLoginScreen();
      return;
    }
    const pair = await apiRefresh(rt);
    await saveTokens({
      accessToken: pair.access_token,
      refreshToken: pair.refresh_token,
      sessionId: pair.session_id,
    });
    log(`refresh OK, new session: ${pair.session_id}`);
    await loadHome();
  } catch (err) {
    if (
      err instanceof SessionCompromisedError ||
      err instanceof SessionExpiredError ||
      err instanceof SessionRevokedError
    ) {
      await clearAllTokens();
      setStatus("Сессия истекла. Войдите заново.", true);
    } else {
      setStatus(String(err), true);
      log(`ERR no-biometric unlock: ${String(err)}`);
    }
    showLoginScreen();
  }
}

async function startUnlock(): Promise<void> {
  showScreen("unlock");
  setStatus("Ожидание биометрии...");
  log("cold start: запрашиваем биометрию...");

  try {
    const rt = await getRefreshToken("Разблокируйте my-chat");
    if (!rt) {
      log("refresh token не найден, переход на Login");
      showLoginScreen();
      return;
    }

    log("биометрия успешна, refresh access token...");
    setStatus("Обновление токенов...");
    const pair = await apiRefresh(rt);
    await saveTokens({
      accessToken: pair.access_token,
      refreshToken: pair.refresh_token,
      sessionId: pair.session_id,
    });
    log(`refresh OK, new session: ${pair.session_id}`);
    await loadHome();
  } catch (err) {
    if (err instanceof SessionCompromisedError) {
      log("session_compromised! Family revoked. Выход.");
      await clearAllTokens();
      setStatus("Сессия скомпрометирована. Войдите заново.", true);
      showLoginScreen();
      return;
    }
    if (err instanceof SessionExpiredError || err instanceof SessionRevokedError) {
      log("сессия истекла или отозвана. Выход.");
      await clearAllTokens();
      setStatus("Сессия истекла. Войдите заново.", true);
      showLoginScreen();
      return;
    }

    // Биометрическая ошибка
    const biometryErr = err as { biometryErrorType?: BiometryErrorType };
    if (biometryErr.biometryErrorType !== undefined) {
      const errType = biometryErr.biometryErrorType;
      log(`биометрия failed: ${errType}`);

      // Смена биометрии / lockout → wipe tokens
      if (
        errType === BiometryErrorType.biometryLockout ||
        errType === BiometryErrorType.biometryNotEnrolled ||
        errType === BiometryErrorType.passcodeNotSet
      ) {
        log("биометрия недоступна, очищаем токены");
        await clearAllTokens();
        setStatus("Биометрия недоступна. Войдите заново.", true);
        showLoginScreen();
        return;
      }

      // Пользователь отменил — показываем кнопку retry
      setStatus("Аутентификация отменена.", true);
      log("пользователь отменил биометрию — можно повторить");
      showScreen("unlock");
      return;
    }

    setStatus(String(err), true);
    log(`ERR unlock: ${String(err)}`);
  }
}

// --- Экран: Login ---

function showLoginScreen(): void {
  disconnectWS();
  showScreen("login");
  setStatus("");
  // Восстановить последний user_id
  getSavedUserId()
    .then((id) => {
      if (id) el<HTMLInputElement>("user-id-input").value = id;
    })
    .catch(() => undefined);
}

async function handleLogin(): Promise<void> {
  const userIdInput = el<HTMLInputElement>("user-id-input");
  const userId = userIdInput.value.trim();
  if (!userId) {
    setStatus("Введите user_id", true);
    return;
  }

  setStatus("Выполняется вход...");
  log(`login: ${userId}`);

  try {
    const result = await apiLogin(userId);
    await saveTokens({
      accessToken: result.access_token,
      refreshToken: result.refresh_token,
      sessionId: result.session_id,
      userId,
    });
    log(`login OK, session: ${result.session_id}`);
    await loadHome();
  } catch (err) {
    setStatus(String(err), true);
    log(`ERR login: ${String(err)}`);
  }
}

// --- Logout ---

async function handleLogout(): Promise<void> {
  disconnectWS();
  setStatus("Выход...");
  log("logout...");
  try {
    // Получаем refresh с биометрией для server-side revoke
    const rt = await getRefreshToken("Подтвердите выход");
    if (rt) {
      await apiLogout(rt);
      log("logout: сессия отозвана на сервере");
    }
  } catch {
    log("logout: server revoke пропущен (best-effort)");
  } finally {
    await clearAllTokens();
    log("токены очищены");
    showLoginScreen();
  }
}

// --- Инициализация ---

async function init(): Promise<void> {
  // Login screen
  el("btn-login").addEventListener("click", () => void handleLogin());
  el("user-id-input").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void handleLogin();
  });

  // Unlock screen
  el("btn-retry-unlock").addEventListener("click", () => void startUnlock());
  el("btn-logout-from-unlock").addEventListener("click", () => void handleLogout());

  // Home screen
  el("btn-logout").addEventListener("click", () => void handleLogout());
  el("btn-refresh-count").addEventListener("click", () => void loadHome());
  el("btn-open-chat").addEventListener("click", () => {
    const dialogId = el<HTMLInputElement>("dialog-id-input").value.trim();
    if (!dialogId) {
      setStatus("Введите Dialog ID", true);
      return;
    }
    void showChat(dialogId);
  });

  // Chat screen
  el("btn-back").addEventListener("click", handleBackFromChat);
  el("btn-send").addEventListener("click", () => void handleSendMessage());
  el("msg-input").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void handleSendMessage();
  });

  // При возврате фокуса — дочитываем накопленные непрочитанные сообщения
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden && currentDialogId) {
      flushPendingMarkRead();
    }
  });

  // Проверка biometric availability
  const biometricOk = await isBiometricAvailable();
  log(`biometric available: ${biometricOk}`);

  // Cold start routing
  const hasRefresh = await hasRefreshToken();
  log(`has refresh token: ${hasRefresh}`);

  if (hasRefresh) {
    if (biometricOk) {
      await startUnlock();
    } else {
      await startUnlockNoBiometric();
    }
  } else {
    showLoginScreen();
  }
}

// Запуск после загрузки DOM
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", () => void init());
} else {
  void init();
}
