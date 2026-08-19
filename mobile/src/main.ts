/**
 * main.ts — точка входа. Управляет состояниями экранов:
 *
 *   [register] ← кнопка «Зарегистрироваться» на экране Login
 *   [login]    ← нет refresh token / session_compromised / logout
 *      ↓ после login/register
 *   [setup-pin] ← PIN ещё не задан на устройстве
 *   [unlock]   ← есть refresh token (cold start)
 *      ↓ биометрия / PIN + refresh
 *   [home]     ← список диалогов (только если PIN задан)
 *      ↓ новый чат / тап по строке / deep link
 *   [new-chat] ← username (+ search)
 *   [chat]     ← открытый диалог
 */

import {
  BiometryErrorType,
  clearAllTokens,
  getRefreshTokenRaw,
  getSavedUserId,
  hasRefreshToken,
  isBiometricAvailable,
  saveTokens,
  setRefreshToken,
  getRefreshToken,
} from "./auth";
import {
  apiLogin,
  apiLogout,
  apiRefresh,
  apiRegister,
  createDialog,
  extractUserIdFromJwt,
  getMessages,
  getUnreadCount,
  getVapidPublicKey,
  listDialogs,
  markRead,
  registerDevice,
  searchUsers,
  sendMessage,
  ApiError,
  PinRequiredError,
  SessionCompromisedError,
  SessionExpiredError,
  SessionRevokedError,
  ensureFreshAccess,
  rotateSession,
  setOnPinRequired,
  type DialogListItem,
  type Message,
} from "./api";
import {
  PIN_LENGTH,
  PIN_LOCK_GRACE_MS,
  PIN_MAX_ATTEMPTS,
  changePin,
  clearPin,
  clearSessionPin,
  encryptRefreshToken,
  isEncryptedRefresh,
  isPinSet,
  isValidPinFormat,
  setSessionPin,
  setupPin,
  verifyPin,
} from "./pin";

/** Счётчик неверных PIN на unlock (lockout → wipe). */
let pinFailCount = 0;

/** Resume lock: UI скрыт, ждём PIN (JWT может остаться в памяти). */
let appLocked = false;
let hiddenAt: number | null = null;
let screenBeforeLock: ScreenName | null = null;
let dialogBeforeLock = "";
let peerBeforeLock = "";

// --- DOM helpers ---

function el<T extends HTMLElement>(id: string): T {
  const e = document.getElementById(id);
  if (!e) throw new Error(`Element #${id} not found`);
  return e as T;
}

type ScreenName =
  | "login"
  | "register"
  | "setup-pin"
  | "unlock"
  | "home"
  | "change-pin"
  | "new-chat"
  | "chat";

const ALL_SCREENS: ScreenName[] = [
  "login",
  "register",
  "setup-pin",
  "unlock",
  "home",
  "change-pin",
  "new-chat",
  "chat",
];

function showScreen(name: ScreenName): void {
  for (const s of ALL_SCREENS) {
    document.getElementById(s)?.classList.toggle("active", s === name);
  }
  // Скрываем статус-бар и лог-панель в режиме чата
  const statusBar = document.getElementById("status-bar");
  const logDetails = document.getElementById("log-details");
  if (statusBar) statusBar.style.display = name === "chat" ? "none" : "";
  if (logDetails) (logDetails as HTMLElement).style.display = name === "chat" ? "none" : "";
}

function getActiveScreen(): ScreenName | null {
  for (const s of ALL_SCREENS) {
    if (document.getElementById(s)?.classList.contains("active")) return s;
  }
  return null;
}

function isAuthenticatedAppScreen(name: ScreenName): boolean {
  return (
    name === "home" ||
    name === "chat" ||
    name === "new-chat" ||
    name === "change-pin"
  );
}

function resetResumeLockState(): void {
  appLocked = false;
  hiddenAt = null;
  screenBeforeLock = null;
  dialogBeforeLock = "";
  peerBeforeLock = "";
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

// --- Сохранение username в localStorage (не секрет) ---

const LS_USERNAME = "my_chat_username";

function saveUsername(username: string): void {
  localStorage.setItem(LS_USERNAME, username);
}

function getSavedUsername(): string {
  return localStorage.getItem(LS_USERNAME) ?? "";
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
    // Пропущенные message_deleted / ttl_started за время разрыва — reload ленты.
    if (currentDialogId && !appLocked && !document.hidden) {
      void loadChatHistory();
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
  try {
    const token = await ensureFreshAccess();
    if (token) connectWS(token);
  } catch (err) {
    if (err instanceof PinRequiredError) {
      log("WS reconnect: ждём PIN");
      return;
    }
    log(`ERR WS reconnect: ${String(err)}`);
  }
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

/** Debounced refresh списка чатов + unread (без смены экрана). */
let homeRefreshTimer: ReturnType<typeof setTimeout> | null = null;

function scheduleHomeRefresh(): void {
  if (homeRefreshTimer !== null) clearTimeout(homeRefreshTimer);
  homeRefreshTimer = setTimeout(() => {
    homeRefreshTimer = null;
    void refreshHomeData();
  }, 400);
}

/**
 * Обновляет список диалогов и badge, не переключая экран.
 * Нужен при message_new, когда открыт Home / другой чат (push при online WS не шлётся).
 */
async function refreshHomeData(): Promise<void> {
  const listEl = document.getElementById("dialogs-list");
  if (!listEl) return;

  try {
    const [count, dialogs] = await Promise.all([getUnreadCount(), listDialogs()]);
    const countEl = document.getElementById("unread-count");
    if (countEl) countEl.textContent = String(count);
    void syncAppBadge(count);
    renderDialogsList(dialogs);
    log(`WS refresh home: dialogs=${dialogs.length}, unread=${count}`);
  } catch (err) {
    if (err instanceof PinRequiredError) return;
    log(`ERR refreshHomeData: ${String(err)}`);
  }
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
      } else {
        // Home / другой чат: push не придёт (WS online) — обновляем список и unread.
        scheduleHomeRefresh();
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
      const bubbleEl = document.querySelector<HTMLElement>(`[data-msg-id="${d.message_id}"]`);
      if (bubbleEl) hideExpiredBubble(d.message_id, bubbleEl);
      else stopTTLTimer(d.message_id);
      // Preview в списке мог ссылаться на удалённое — подтянуть актуальное.
      if (!currentDialogId || d.dialog_id !== currentDialogId) {
        scheduleHomeRefresh();
      }
      break;
    }
    case "badge_updated": {
      const d = env.data as WSDataBadgeUpdated;
      const countEl = document.getElementById("unread-count");
      if (countEl) countEl.textContent = String(d.unread_count);
      void syncAppBadge(d.unread_count);
      break;
    }
    default:
      break;
  }
}

// --- TTL timers ---

const ttlTimers = new Map<string, ReturnType<typeof setInterval>>();

function hideExpiredBubble(messageId: string, bubbleEl: HTMLElement): void {
  stopTTLTimer(messageId);
  bubbleEl.style.display = "none";
}

/**
 * Визуальный обратный отсчёт. По истечении expires_at пузырь скрывается локально
 * (не ждём message_deleted — событие теряется, если PWA была в фоне).
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
      hideExpiredBubble(messageId, bubbleEl);
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

// --- App icon badge (Badging API, iOS PWA / installed web apps) ---

/**
 * Синхронизирует бейдж на иконке с числом непрочитанных.
 * Важно: при открытом чате badge_updated приходит по WS — без этого
 * вызова иконка остаётся со старым числом (push badge_sync на iOS
 * с userVisibleOnly ненадёжен без showNotification).
 */
async function syncAppBadge(count: number): Promise<void> {
  const nav = navigator as Navigator & {
    setAppBadge?: (n?: number) => Promise<void>;
    clearAppBadge?: () => Promise<void>;
  };
  try {
    if (count > 0) {
      if (typeof nav.setAppBadge === "function") {
        await nav.setAppBadge(count);
      }
      return;
    }
    if (typeof nav.clearAppBadge === "function") {
      await nav.clearAppBadge();
    } else if (typeof nav.setAppBadge === "function") {
      await nav.setAppBadge(0);
    }
  } catch {
    // Unsupported / permission — ignore
  }
}

// --- Page Visibility / markRead / resume lock ---
//
// markRead вызывается только когда диалог видим и UI не locked.
// Если вкладка скрыта или appLocked — ID в pendingMarkRead.
// Resume: после grace (PIN_LOCK_GRACE_MS) → Unlock PIN, контент скрыт.

const pendingMarkRead = new Set<string>();

function tryMarkRead(messageId: string): void {
  if (!document.hidden && !appLocked) {
    void markRead(messageId).catch(() => undefined);
  } else {
    pendingMarkRead.add(messageId);
  }
}

function flushPendingMarkRead(): void {
  if (appLocked) return;
  if (pendingMarkRead.size === 0) return;
  const ids = [...pendingMarkRead];
  pendingMarkRead.clear();
  for (const id of ids) {
    void markRead(id).catch(() => undefined);
  }
}

function onAppHidden(): void {
  if (appLocked) return;
  const active = getActiveScreen();
  if (!active || !isAuthenticatedAppScreen(active)) return;
  hiddenAt = Date.now();
}

async function lockAppForResume(): Promise<void> {
  const active = getActiveScreen();
  if (!active || !isAuthenticatedAppScreen(active)) return;
  if (!(await isPinSet())) return;

  screenBeforeLock = active;
  dialogBeforeLock = currentDialogId;
  peerBeforeLock = currentPeerUsername;
  appLocked = true;
  log(`resume lock (grace ${PIN_LOCK_GRACE_MS}ms exceeded)`);
  clearSessionPin();
  await startUnlockWithPin();
}

async function unlockAfterResume(): Promise<void> {
  const target = screenBeforeLock ?? "home";
  const dialogId = dialogBeforeLock;
  const peer = peerBeforeLock;
  resetResumeLockState();
  setStatus("");
  log(`resume unlock OK → ${target}`);

  if (target === "chat" && dialogId) {
    currentDialogId = dialogId;
    currentPeerUsername = peer;
    const titleEl = document.getElementById("chat-title");
    if (titleEl) titleEl.textContent = peer || "Чат";
    showScreen("chat");
    await loadChatHistory();
    flushPendingMarkRead();
    return;
  }
  if (target === "new-chat") {
    showNewChatScreen();
    return;
  }
  await loadHome();
}

async function onAppVisible(): Promise<void> {
  if (appLocked) {
    // Уже на unlock — не flush mark_read, не показываем chat/home
    return;
  }

  const hiddenFor = hiddenAt !== null ? Date.now() - hiddenAt : 0;
  hiddenAt = null;

  if (hiddenFor >= PIN_LOCK_GRACE_MS) {
    await lockAppForResume();
    if (appLocked) return;
  }

  if (currentDialogId) {
    await loadChatHistory();
    flushPendingMarkRead();
    return;
  }
  if (getActiveScreen() === "home") {
    await refreshHomeData();
  }
}

// --- Chat screen ---

let currentUserId = "";
let currentDialogId = "";
let currentPeerUsername = "";

/** dialog_id → peer username (кэш для заголовка / deep link). */
const dialogPeers = new Map<string, string>();

/** Отложенный dialog_id из /?dialog= или SW open_dialog до готовности сессии. */
let pendingDialogId = "";

function escHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function rememberDialogPeer(dialogId: string, username: string): void {
  if (dialogId && username) dialogPeers.set(dialogId, username);
}

function takePendingDialogFromUrl(): void {
  const params = new URLSearchParams(window.location.search);
  const fromUrl = params.get("dialog")?.trim() ?? "";
  if (fromUrl) {
    pendingDialogId = fromUrl;
    window.history.replaceState({}, "", window.location.pathname);
  }
}

async function openPendingDialogIfAny(): Promise<void> {
  if (!pendingDialogId) return;
  const id = pendingDialogId;
  pendingDialogId = "";
  await showChat(id, dialogPeers.get(id));
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

async function showChat(dialogId: string, peerUsername?: string): Promise<void> {
  currentDialogId = dialogId;
  currentPeerUsername = peerUsername || dialogPeers.get(dialogId) || "";
  if (currentPeerUsername) rememberDialogPeer(dialogId, currentPeerUsername);

  const titleEl = document.getElementById("chat-title");
  if (titleEl) {
    titleEl.textContent = currentPeerUsername || "Чат";
  }

  showScreen("chat");
  await loadChatHistory();
}

/** Отбрасывает устаревший ответ, если за время GET открыли другой чат / начали новый reload. */
let chatHistoryGen = 0;

async function loadChatHistory(): Promise<void> {
  if (!currentDialogId) return;
  const dialogId = currentDialogId;
  const gen = ++chatHistoryGen;

  stopAllTTLTimers();
  el("messages-list").innerHTML = "";

  try {
    const messages = await getMessages(dialogId);
    if (gen !== chatHistoryGen || currentDialogId !== dialogId) return;

    messages.reverse(); // API возвращает DESC, рендерим старые→сверху, новые→снизу
    const now = Date.now();
    for (const msg of messages) {
      // Сообщение уже истекло по TTL — не рендерим.
      if (msg.expires_at && new Date(msg.expires_at).getTime() <= now) {
        continue;
      }

      const bubbleEl = appendBubble(msg);

      if (msg.expires_at) {
        startTTLTimer(msg.id, msg.expires_at, bubbleEl);
      } else if (msg.sender_id !== currentUserId) {
        tryMarkRead(msg.id);
      }
    }
    scrollToBottom();
  } catch (err) {
    if (err instanceof PinRequiredError) return;
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
    if (err instanceof PinRequiredError) return;
    log(`ERR sendMessage: ${String(err)}`);
  }
}

function handleBackFromChat(): void {
  stopAllTTLTimers();
  pendingMarkRead.clear();
  currentDialogId = "";
  currentPeerUsername = "";
  void loadHome();
}

// --- Экран: Home ---

function renderDialogsList(dialogs: DialogListItem[]): void {
  const listEl = el("dialogs-list");
  const emptyEl = el("dialogs-empty");
  listEl.innerHTML = "";

  if (dialogs.length === 0) {
    listEl.style.display = "none";
    emptyEl.style.display = "";
    return;
  }

  listEl.style.display = "";
  emptyEl.style.display = "none";

  for (const d of dialogs) {
    rememberDialogPeer(d.dialog_id, d.peer.username);

    const row = document.createElement("button");
    row.type = "button";
    row.className = "dialog-row";
    row.dataset.dialogId = d.dialog_id;

    const preview = d.last_message?.body_preview?.trim() || "Нет сообщений";
    const unread =
      d.unread_count > 0
        ? `<span class="dialog-unread">${d.unread_count > 99 ? "99+" : d.unread_count}</span>`
        : "";

    row.innerHTML = `
      <div class="dialog-row-main">
        <div class="dialog-row-name">${escHtml(d.peer.username)}</div>
        <div class="dialog-row-preview">${escHtml(preview)}</div>
      </div>
      ${unread}
    `;
    row.addEventListener("click", () => {
      void showChat(d.dialog_id, d.peer.username);
    });
    listEl.appendChild(row);
  }
}

function showNewChatScreen(): void {
  showScreen("new-chat");
  setStatus("");
  const input = el<HTMLInputElement>("new-chat-username");
  input.value = "";
  el("search-results").innerHTML = "";
  input.focus();
}

// --- Settings: Change PIN ---

function showChangePinScreen(): void {
  showScreen("change-pin");
  setStatus("");
  for (const id of ["change-pin-old", "change-pin-new", "change-pin-confirm"]) {
    el<HTMLInputElement>(id).value = "";
  }
  el<HTMLInputElement>("change-pin-old").focus();
}

async function handleChangePin(): Promise<void> {
  const oldPin = digitsOnly(el<HTMLInputElement>("change-pin-old").value);
  const newPin = digitsOnly(el<HTMLInputElement>("change-pin-new").value);
  const confirm = digitsOnly(el<HTMLInputElement>("change-pin-confirm").value);

  if (!isValidPinFormat(oldPin)) {
    setStatus(`Текущий PIN: ${PIN_LENGTH} цифры`, true);
    return;
  }
  if (!isValidPinFormat(newPin)) {
    setStatus(`Новый PIN должен состоять из ${PIN_LENGTH} цифр`, true);
    return;
  }
  if (newPin !== confirm) {
    setStatus("Новый PIN не совпадает", true);
    return;
  }
  if (oldPin === newPin) {
    setStatus("Новый PIN совпадает со старым", true);
    return;
  }

  setStatus("Смена PIN...");
  log("change PIN...");
  try {
    const storedRefresh = await getRefreshTokenRaw();
    const result = await changePin(oldPin, newPin, storedRefresh);
    if (!result.ok) {
      if (result.reason === "invalid_old") {
        setStatus("Неверный текущий PIN", true);
      } else {
        setStatus(`Новый PIN должен состоять из ${PIN_LENGTH} цифр`, true);
      }
      return;
    }
    if (result.refreshBlob) {
      await setRefreshToken(result.refreshBlob);
      log("refresh перешифрован новым PIN");
    }
    setSessionPin(newPin);
    setStatus("PIN обновлён");
    log("change PIN OK → Home");
    await loadHome();
  } catch (err) {
    setStatus(String(err), true);
    log(`ERR change PIN: ${String(err)}`);
  }
}

let searchTimer: ReturnType<typeof setTimeout> | null = null;

function scheduleUserSearch(): void {
  if (searchTimer !== null) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => void runUserSearch(), 250);
}

async function runUserSearch(): Promise<void> {
  const q = el<HTMLInputElement>("new-chat-username").value.trim();
  const resultsEl = el("search-results");
  resultsEl.innerHTML = "";
  if (q.length < 2) return;

  try {
    const users = await searchUsers(q);
    for (const u of users) {
      const li = document.createElement("li");
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = u.username;
      btn.addEventListener("click", () => {
        el<HTMLInputElement>("new-chat-username").value = u.username;
        resultsEl.innerHTML = "";
        void handleCreateDialog();
      });
      li.appendChild(btn);
      resultsEl.appendChild(li);
    }
  } catch (err) {
    if (err instanceof PinRequiredError) return;
    log(`ERR searchUsers: ${String(err)}`);
  }
}

async function handleCreateDialog(): Promise<void> {
  const username = el<HTMLInputElement>("new-chat-username").value.trim();
  if (!username) {
    setStatus("Введите username", true);
    return;
  }

  setStatus("Создание чата...");
  try {
    const item = await createDialog(username);
    rememberDialogPeer(item.dialog_id, item.peer.username);
    log(`POST /dialogs → ${item.dialog_id} peer=${item.peer.username}`);
    setStatus("");
    await showChat(item.dialog_id, item.peer.username);
  } catch (err) {
    if (err instanceof PinRequiredError) return;
    if (
      err instanceof SessionCompromisedError ||
      err instanceof SessionExpiredError ||
      err instanceof SessionRevokedError
    ) {
      await clearAllTokens();
      showLoginScreen();
      return;
    }
    const msg = err instanceof ApiError || err instanceof Error ? err.message : String(err);
    setStatus(msg, true);
    log(`ERR createDialog: ${msg}`);
  }
}

async function loadHome(): Promise<void> {
  // Gate: без PIN на Home нельзя (login/register/cold-start миграция)
  if (!(await isPinSet())) {
    log("loadHome: PIN не задан → Setup PIN");
    showSetupPinScreen();
    return;
  }
  showScreen("home");
  setStatus("Загрузка...");

  currentUserId = (await getSavedUserId()) ?? "";

  // Подключаем WS если ещё не подключены
  try {
    const token = await ensureFreshAccess();
    if (token) connectWS(token);
  } catch (err) {
    if (!(err instanceof PinRequiredError)) {
      log(`ERR WS connect: ${String(err)}`);
    }
  }

  // Подписка на Web Push (best-effort, не блокирует загрузку home)
  void subscribePush();

  try {
    const [count, dialogs] = await Promise.all([getUnreadCount(), listDialogs()]);
    el("unread-count").textContent = String(count);
    void syncAppBadge(count);
    renderDialogsList(dialogs);
    setStatus(dialogs.length === 0 ? "Нет чатов" : "Загружено");
    log(`GET /dialogs → ${dialogs.length}; unread → ${count}`);
    await openPendingDialogIfAny();
  } catch (err) {
    if (err instanceof PinRequiredError) return;
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

function setUnlockUiMode(mode: "pin" | "biometric"): void {
  const pinBlock = document.getElementById("unlock-pin-block");
  const retryBtn = document.getElementById("btn-retry-unlock");
  const subtitle = document.getElementById("unlock-subtitle");
  if (pinBlock) pinBlock.style.display = mode === "pin" ? "" : "none";
  if (retryBtn) retryBtn.style.display = mode === "biometric" ? "" : "none";
  if (subtitle) {
    subtitle.textContent =
      mode === "pin"
        ? "Введите PIN для разблокировки"
        : "Подтвердите личность для разблокировки";
  }
}

/** Silent refresh + persist; optionally re-encrypt new refresh with PIN. */
async function finishUnlockWithPlainRefresh(
  plainRefresh: string,
  pinForReencrypt?: string
): Promise<void> {
  setStatus("Обновление сессии...");
  const pair = await apiRefresh(plainRefresh);
  await saveTokens({
    accessToken: pair.access_token,
    refreshToken: pair.refresh_token,
    sessionId: pair.session_id,
  });
  if (pinForReencrypt) {
    const blob = await encryptRefreshToken(pinForReencrypt, pair.refresh_token);
    await setRefreshToken(blob);
  }
  log(`refresh OK, new session: ${pair.session_id}`);
  pinFailCount = 0;
  await loadHome();
}

async function handleUnlockAuthError(err: unknown): Promise<boolean> {
  if (err instanceof SessionCompromisedError) {
    log("session_compromised! Family revoked. Выход.");
    await clearAllTokens();
    await clearPin();
    setStatus("Сессия скомпрометирована. Войдите заново.", true);
    showLoginScreen();
    return true;
  }
  if (err instanceof SessionExpiredError || err instanceof SessionRevokedError) {
    log("сессия истекла или отозвана. Выход.");
    await clearAllTokens();
    await clearPin();
    setStatus("Сессия истекла. Войдите заново.", true);
    showLoginScreen();
    return true;
  }
  return false;
}

/**
 * PWA / no-biometric: экран ввода PIN (вместо silent auto-login).
 */
async function startUnlockWithPin(): Promise<void> {
  showScreen("unlock");
  setUnlockUiMode("pin");
  setStatus("");
  const pinInput = el<HTMLInputElement>("unlock-pin-input");
  pinInput.value = "";
  pinInput.focus();
  log("unlock: ожидание PIN");
}

/** Чтобы input + Enter не запускали verify дважды, пока идёт проверка. */
let unlockPinBusy = false;

async function handleUnlockPin(): Promise<void> {
  if (unlockPinBusy) return;

  const pinInput = el<HTMLInputElement>("unlock-pin-input");
  const pin = digitsOnly(pinInput.value);

  if (!isValidPinFormat(pin)) {
    return;
  }

  unlockPinBusy = true;
  try {
    const ok = await verifyPin(pin);
    if (!ok) {
      pinFailCount += 1;
      log(`unlock PIN fail ${pinFailCount}/${PIN_MAX_ATTEMPTS}`);
      pinInput.value = "";
      if (pinFailCount >= PIN_MAX_ATTEMPTS) {
        pinFailCount = 0;
        await clearAllTokens();
        await clearPin();
        resetResumeLockState();
        setStatus("Слишком много попыток. Войдите заново.", true);
        showLoginScreen();
        return;
      }
      setStatus(
        `Неверный PIN. Осталось попыток: ${PIN_MAX_ATTEMPTS - pinFailCount}`,
        true
      );
      pinInput.focus();
      return;
    }

    pinFailCount = 0;
    pinInput.value = "";
    setSessionPin(pin);

    setStatus("Обновление сессии...");
    log("unlock PIN OK → refresh");
    try {
      await rotateSession();
    } catch (err) {
      if (await handleUnlockAuthError(err)) return;
      setStatus(String(err), true);
      log(`ERR unlock PIN: ${String(err)}`);
      return;
    }

    if (appLocked && screenBeforeLock) {
      await unlockAfterResume();
      return;
    }

    await loadHome();
  } finally {
    unlockPinBusy = false;
  }
}

/**
 * Capacitor native: Face ID как раньше.
 * Если refresh уже ciphertext — после биометрии просим PIN для decrypt.
 */
async function startUnlock(): Promise<void> {
  showScreen("unlock");
  setUnlockUiMode("biometric");
  setStatus("Ожидание биометрии...");
  log("cold start: запрашиваем биометрию...");

  try {
    const rt = await getRefreshToken("Разблокируйте my-chat");
    if (!rt) {
      log("refresh token не найден, переход на Login");
      showLoginScreen();
      return;
    }

    if (isEncryptedRefresh(rt)) {
      log("биометрия OK, refresh зашифрован → PIN");
      setUnlockUiMode("pin");
      setStatus("Введите PIN");
      const pinInput = el<HTMLInputElement>("unlock-pin-input");
      pinInput.value = "";
      pinInput.focus();
      return;
    }

    log("биометрия успешна, refresh access token...");
    await finishUnlockWithPlainRefresh(rt);
  } catch (err) {
    if (await handleUnlockAuthError(err)) return;

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
        await clearPin();
        setStatus("Биометрия недоступна. Войдите заново.", true);
        showLoginScreen();
        return;
      }

      // Пользователь отменил — кнопка Face ID retry
      setStatus("Аутентификация отменена.", true);
      log("пользователь отменил биометрию — можно повторить");
      setUnlockUiMode("biometric");
      showScreen("unlock");
      return;
    }

    setStatus(String(err), true);
    log(`ERR unlock: ${String(err)}`);
  }
}

// --- Экран: Setup PIN ---

function digitsOnly(value: string): string {
  return value.replace(/\D/g, "").slice(0, PIN_LENGTH);
}

function bindPinInput(id: string, onComplete?: () => void): void {
  const input = el<HTMLInputElement>(id);
  input.addEventListener("input", () => {
    const next = digitsOnly(input.value);
    if (input.value !== next) input.value = next;
    if (onComplete && next.length === PIN_LENGTH) onComplete();
  });
}

function showSetupPinScreen(): void {
  showScreen("setup-pin");
  setStatus("");
  const pinInput = el<HTMLInputElement>("setup-pin-input");
  const confirmInput = el<HTMLInputElement>("setup-pin-confirm");
  pinInput.value = "";
  confirmInput.value = "";
  pinInput.focus();
}

/** После успешного login/register: Setup PIN или Home. */
async function enterAppAfterAuth(): Promise<void> {
  if (!(await isPinSet())) {
    log("PIN не задан → Setup PIN");
    showSetupPinScreen();
    return;
  }
  await loadHome();
}

async function handleSetupPin(): Promise<void> {
  const pinInput = el<HTMLInputElement>("setup-pin-input");
  const confirmInput = el<HTMLInputElement>("setup-pin-confirm");
  const pin = digitsOnly(pinInput.value);
  const confirm = digitsOnly(confirmInput.value);

  if (!isValidPinFormat(pin)) {
    setStatus(`PIN должен состоять из ${PIN_LENGTH} цифр`, true);
    return;
  }
  if (pin !== confirm) {
    setStatus("PIN не совпадает", true);
    return;
  }

  setStatus("Сохранение PIN...");
  log("setup PIN...");
  try {
    await setupPin(pin);

    // Should: зашифровать refresh ключом из PIN
    const rt = await getRefreshTokenRaw();
    if (rt && !isEncryptedRefresh(rt)) {
      const blob = await encryptRefreshToken(pin, rt);
      await setRefreshToken(blob);
      log("refresh token зашифрован PIN");
    }

    pinInput.value = "";
    confirmInput.value = "";
    setSessionPin(pin);
    setStatus("");
    log("setup PIN OK → Home");
    await loadHome();
  } catch (err) {
    setStatus(String(err), true);
    log(`ERR setup PIN: ${String(err)}`);
  }
}

// --- Экран: Login ---

function showLoginScreen(): void {
  disconnectWS();
  clearSessionPin();
  showScreen("login");
  setStatus("");
  // Восстановить последний username
  const saved = getSavedUsername();
  if (saved) {
    const usernameInput = document.getElementById("username-input") as HTMLInputElement | null;
    if (usernameInput) usernameInput.value = saved;
  }
}

async function handleLogin(): Promise<void> {
  const usernameInput = el<HTMLInputElement>("username-input");
  const passwordInput = el<HTMLInputElement>("password-input");
  const username = usernameInput.value.trim();
  const password = passwordInput.value;

  if (!username) { setStatus("Введите имя пользователя", true); return; }
  if (!password) { setStatus("Введите пароль", true); return; }

  setStatus("Выполняется вход...");
  log(`login: ${username}`);

  try {
    const result = await apiLogin(username, password);
    const userId = extractUserIdFromJwt(result.access_token);

    await saveTokens({
      accessToken: result.access_token,
      refreshToken: result.refresh_token,
      sessionId: result.session_id,
      userId,
    });
    saveUsername(username);
    passwordInput.value = "";
    log(`login OK, user: ${userId}, session: ${result.session_id}`);
    await enterAppAfterAuth();
  } catch (err) {
    setStatus(String(err), true);
    log(`ERR login: ${String(err)}`);
  }
}

// --- Экран: Register ---

function showRegisterScreen(): void {
  showScreen("register");
  setStatus("");
}

async function handleRegister(): Promise<void> {
  const usernameInput = el<HTMLInputElement>("reg-username-input");
  const passwordInput = el<HTMLInputElement>("reg-password-input");
  const confirmInput  = el<HTMLInputElement>("reg-confirm-input");

  const username = usernameInput.value.trim();
  const password = passwordInput.value;
  const confirm  = confirmInput.value;

  if (!username) { setStatus("Введите имя пользователя", true); return; }
  if (password.length < 8) { setStatus("Пароль должен содержать не менее 8 символов", true); return; }
  if (password !== confirm) { setStatus("Пароли не совпадают", true); return; }

  setStatus("Создание аккаунта...");
  log(`register: ${username}`);

  try {
    await apiRegister(username, password);
    log(`register OK: ${username}`);
    // Сразу login → Setup PIN (не пускать на Home без PIN)
    setStatus("Вход...");
    const result = await apiLogin(username, password);
    const userId = extractUserIdFromJwt(result.access_token);
    await saveTokens({
      accessToken: result.access_token,
      refreshToken: result.refresh_token,
      sessionId: result.session_id,
      userId,
    });
    saveUsername(username);
    passwordInput.value = "";
    confirmInput.value = "";
    log(`register→login OK, user: ${userId}`);
    await enterAppAfterAuth();
  } catch (err) {
    setStatus(String(err), true);
    log(`ERR register: ${String(err)}`);
  }
}

// --- Logout ---

async function handleLogout(): Promise<void> {
  disconnectWS();
  setStatus("Выход...");
  log("logout...");
  try {
    const biometricOk = await isBiometricAvailable();
    const rt = biometricOk
      ? await getRefreshToken("Подтвердите выход")
      : await getRefreshTokenRaw();
    if (rt) {
      // ciphertext нельзя revoke без PIN; plaintext — как раньше
      if (!isEncryptedRefresh(rt)) {
        await apiLogout(rt);
        log("logout: сессия отозвана на сервере");
      } else {
        log("logout: refresh зашифрован — server revoke пропущен");
      }
    }
  } catch {
    log("logout: server revoke пропущен (best-effort)");
  } finally {
    await clearAllTokens();
    await clearPin();
    resetResumeLockState();
    pinFailCount = 0;
    log("токены и PIN очищены");
    showLoginScreen();
  }
}

// --- Web Push ---

/**
 * Конвертирует base64url VAPID public key в Uint8Array для pushManager.subscribe.
 */
function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const buffer = new ArrayBuffer(raw.length);
  const bytes = new Uint8Array(buffer);
  for (let i = 0; i < raw.length; i++) {
    bytes[i] = raw.charCodeAt(i);
  }
  return bytes;
}

/**
 * Запрашивает разрешение и подписывается на Web Push.
 * Best-effort: ошибки логируются, но не останавливают загрузку home.
 */
async function subscribePush(): Promise<void> {
  // Fallback: iOS < 16.4 и другие браузеры без Push API
  if (!("PushManager" in window)) {
    log("Push API недоступен в этом браузере (iOS < 16.4?)");
    return;
  }

  if (!("serviceWorker" in navigator)) return;

  try {
    // Запрашиваем разрешение на уведомления
    const permission = await Notification.requestPermission();
    if (permission !== "granted") {
      log(`Push: разрешение не получено (${permission})`);
      return;
    }

    const reg = await navigator.serviceWorker.ready;

    // Если уже подписаны — не пересоздаём подписку
    const existing = await reg.pushManager.getSubscription();
    if (existing) {
      log("Push: уже подписаны, обновляем регистрацию устройства");
      await registerDevice("web", existing.toJSON());
      return;
    }

    const vapidKey = await getVapidPublicKey();
    if (!vapidKey) {
      log("Push: VAPID ключ не получен, подписка пропущена");
      return;
    }

    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidKey),
    });

    await registerDevice("web", sub.toJSON());
    log("Push: подписка успешна");
  } catch (err) {
    if (err instanceof PinRequiredError) return;
    log(`Push: ошибка подписки — ${String(err)}`);
  }
}

// --- PWA: Service Worker + Install banner ---

let deferredInstallPrompt: BeforeInstallPromptEvent | null = null;

interface BeforeInstallPromptEvent extends Event {
  prompt(): Promise<void>;
  readonly userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
}

function initPWA(): void {
  if ("serviceWorker" in navigator) {
    navigator.serviceWorker
      .register("/sw.js")
      .then((reg) => { log(`SW зарегистрирован: ${reg.scope}`); })
      .catch((err) => { log(`SW ошибка регистрации: ${String(err)}`); });

    // Обработка postMessage от SW (open_dialog при клике на уведомление)
    navigator.serviceWorker.addEventListener("message", (ev) => {
      const msg = ev.data as { type?: string; dialog_id?: string } | undefined;
      if (msg?.type === "open_dialog" && msg.dialog_id) {
        if (currentUserId) {
          void showChat(msg.dialog_id, dialogPeers.get(msg.dialog_id));
        } else {
          pendingDialogId = msg.dialog_id;
        }
      }
    });
  }

  const isStandalone = window.matchMedia("(display-mode: standalone)").matches;
  if (isStandalone) return;

  window.addEventListener("beforeinstallprompt", (ev) => {
    ev.preventDefault();
    deferredInstallPrompt = ev as BeforeInstallPromptEvent;
    showInstallBanner();
  });

  window.addEventListener("appinstalled", () => {
    hideInstallBanner();
    deferredInstallPrompt = null;
    log("PWA установлена");
  });
}

function showInstallBanner(): void {
  document.getElementById("install-banner")?.classList.add("visible");
}

function hideInstallBanner(): void {
  document.getElementById("install-banner")?.classList.remove("visible");
}

function initInstallBannerButtons(): void {
  document.getElementById("btn-install")?.addEventListener("click", () => {
    if (!deferredInstallPrompt) return;
    hideInstallBanner();
    void deferredInstallPrompt.prompt().then(() => {
      void deferredInstallPrompt!.userChoice.then((choice) => {
        log(`PWA install: ${choice.outcome}`);
        deferredInstallPrompt = null;
      });
    });
  });

  document.getElementById("btn-install-dismiss")?.addEventListener("click", () => {
    hideInstallBanner();
  });
}

// --- Инициализация ---

async function init(): Promise<void> {
  // Login screen
  el("btn-login").addEventListener("click", () => void handleLogin());
  el("username-input").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") el<HTMLInputElement>("password-input").focus();
  });
  el("password-input").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void handleLogin();
  });
  el("btn-go-register").addEventListener("click", showRegisterScreen);

  // Register screen
  el("btn-register").addEventListener("click", () => void handleRegister());
  el("btn-back-to-login").addEventListener("click", showLoginScreen);
  el("reg-confirm-input").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void handleRegister();
  });

  // Setup PIN screen
  bindPinInput("setup-pin-input");
  bindPinInput("setup-pin-confirm");
  el("btn-setup-pin").addEventListener("click", () => void handleSetupPin());
  el("btn-logout-from-setup-pin").addEventListener("click", () => void handleLogout());
  el("setup-pin-input").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") el<HTMLInputElement>("setup-pin-confirm").focus();
  });
  el("setup-pin-confirm").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void handleSetupPin();
  });

  // Unlock screen: разблокировка при вводе 4-й цифры (без кнопки)
  bindPinInput("unlock-pin-input", () => void handleUnlockPin());
  el("unlock-pin-input").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void handleUnlockPin();
  });
  el("btn-retry-unlock").addEventListener("click", () => void startUnlock());
  el("btn-logout-from-unlock").addEventListener("click", () => void handleLogout());

  // Home screen
  el("btn-logout").addEventListener("click", () => void handleLogout());
  el("btn-change-pin").addEventListener("click", showChangePinScreen);
  el("btn-refresh-count").addEventListener("click", () => void loadHome());
  el("btn-new-chat").addEventListener("click", showNewChatScreen);
  el("btn-new-chat-empty").addEventListener("click", showNewChatScreen);

  // Change PIN (Settings)
  bindPinInput("change-pin-old");
  bindPinInput("change-pin-new");
  bindPinInput("change-pin-confirm");
  el("btn-change-pin-save").addEventListener("click", () => void handleChangePin());
  el("btn-change-pin-back").addEventListener("click", () => void loadHome());
  el("change-pin-old").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") el<HTMLInputElement>("change-pin-new").focus();
  });
  el("change-pin-new").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") el<HTMLInputElement>("change-pin-confirm").focus();
  });
  el("change-pin-confirm").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void handleChangePin();
  });

  // New chat screen
  el("btn-create-dialog").addEventListener("click", () => void handleCreateDialog());
  el("btn-new-chat-back").addEventListener("click", () => void loadHome());
  el("new-chat-username").addEventListener("input", scheduleUserSearch);
  el("new-chat-username").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void handleCreateDialog();
  });

  // Chat screen
  el("btn-back").addEventListener("click", handleBackFromChat);
  el("btn-send").addEventListener("click", () => void handleSendMessage());
  el("msg-input").addEventListener("keydown", (e) => {
    if ((e as KeyboardEvent).key === "Enter") void handleSendMessage();
  });

  // Background / resume: grace period → PIN lock; иначе flush mark_read
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) {
      onAppHidden();
    } else {
      void onAppVisible();
    }
  });
  // Safari / PWA: pagehide как доп. сигнал ухода в background
  window.addEventListener("pagehide", () => {
    onAppHidden();
  });

  // Кнопки PWA-баннера
  initInstallBannerButtons();

  setOnPinRequired(() => {
    if (appLocked) return;
    if (getActiveScreen() === "unlock") return;
    log("API: нужен PIN для refresh");
    void startUnlockWithPin();
  });

  // Deep link /?dialog= (SW notificationclick / openWindow)
  takePendingDialogFromUrl();

  // Проверка biometric availability
  const biometricOk = await isBiometricAvailable();
  log(`biometric available: ${biometricOk}`);

  // Cold start routing
  const hasRefresh = await hasRefreshToken();
  const pinSet = await isPinSet();
  log(`has refresh token: ${hasRefresh}; pin set: ${pinSet}`);

  if (hasRefresh) {
    if (!pinSet) {
      // Миграция сессий до Sprint 8: есть refresh, PIN ещё не задан
      log("миграция: refresh без PIN → Setup PIN");
      showSetupPinScreen();
    } else if (biometricOk) {
      await startUnlock();
    } else {
      await startUnlockWithPin();
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

// PWA: Service Worker можно зарегистрировать сразу — не зависит от DOM
initPWA();
