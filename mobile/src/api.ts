/**
 * api.ts — HTTP client.
 *
 * Все запросы к main-service идут через fetchAuth() который:
 *   1. Добавляет Authorization + X-Device-ID заголовки.
 *   2. При 401 — пробует refresh (с биометрией).
 *   3. Повторяет запрос с новым access токеном.
 *   4. При session_compromised — wipe tokens + бросает SessionCompromisedError.
 */

import {
  clearAllTokens,
  getAccessToken,
  getRefreshToken,
  saveTokens,
} from "./auth";

// URL бэкенда берётся из Vite env-переменных:
//   .env.development  → пустые строки → relative URLs → Vite proxy
//   .env              → реальные хосты → Capacitor / симулятор
const BASE_AUTH = (import.meta.env.VITE_AUTH_URL as string) ?? "";
const BASE_API = (import.meta.env.VITE_API_URL as string) ?? "";

function authUrl(): string { return BASE_AUTH; }
function apiUrl(): string  { return BASE_API; }

// --- Device ID (localStorage, not sensitive) ---

const LS_DEVICE_ID = "my_chat_device_id";

/** Возвращает постоянный device_id, генерируя его при первом вызове. */
export function getOrCreateDeviceId(): string {
  let id = localStorage.getItem(LS_DEVICE_ID);
  if (!id) {
    id = crypto.randomUUID();
    localStorage.setItem(LS_DEVICE_ID, id);
  }
  return id;
}

// --- JWT utils ---

/**
 * Декодирует payload JWT без верификации подписи.
 * Используется только для извлечения user_id на клиентской стороне.
 */
export function extractUserIdFromJwt(token: string): string {
  try {
    const [, payload] = token.split(".");
    const json = atob(payload.replace(/-/g, "+").replace(/_/g, "/"));
    const claims = JSON.parse(json) as Record<string, unknown>;
    return (claims["user_id"] as string) ?? "";
  } catch {
    return "";
  }
}

// --- Ошибки ---

export class SessionCompromisedError extends Error {
  constructor() {
    super("session_compromised: all sessions revoked");
    this.name = "SessionCompromisedError";
  }
}

export class SessionExpiredError extends Error {
  constructor() {
    super("session_expired");
    this.name = "SessionExpiredError";
  }
}

export class SessionRevokedError extends Error {
  constructor() {
    super("session_revoked");
    this.name = "SessionRevokedError";
  }
}

// --- Типы ---

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  session_id: string;
  expires_in: number;
}

export interface LoginResult extends TokenPair {
  token_type: string;
}

// --- Auth API ---

/** Аутентификация по username + password. */
export async function apiLogin(
  username: string,
  password: string
): Promise<LoginResult> {
  const res = await fetch(`${authUrl()}/api/v1/auth/login`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Device-ID": getOrCreateDeviceId(),
    },
    body: JSON.stringify({ username, password }),
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    const code = (err as { error?: { code?: string } })?.error?.code ?? "";
    if (code === "invalid_credentials") throw new Error("Неверный логин или пароль");
    throw new Error(`login failed: ${res.status} ${JSON.stringify(err)}`);
  }

  return res.json() as Promise<LoginResult>;
}

/** Обновление токена с device binding. */
export async function apiRefresh(refreshToken: string): Promise<TokenPair> {
  const res = await fetch(`${authUrl()}/api/v1/auth/refresh`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Device-ID": getOrCreateDeviceId(),
    },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: {} }));
    const code = (body as { error?: { code?: string } })?.error?.code ?? "";
    if (code === "session_compromised") throw new SessionCompromisedError();
    if (code === "session_expired") throw new SessionExpiredError();
    if (code === "session_revoked") throw new SessionRevokedError();
    throw new Error(`refresh failed: ${res.status}`);
  }

  return res.json() as Promise<TokenPair>;
}

export async function apiLogout(refreshToken: string): Promise<void> {
  await fetch(`${authUrl()}/api/v1/auth/logout`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
  }).catch(() => {
    // best-effort: logout всегда очищает локальные токены
  });
}

/** Регистрация нового пользователя. */
export async function apiRegister(
  username: string,
  password: string
): Promise<void> {
  const res = await fetch(`${apiUrl()}/api/v1/users/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: {} }));
    const code = (body as { error?: { code?: string } })?.error?.code ?? "";
    const msg = (body as { error?: { message?: string } })?.error?.message ?? "";
    if (code === "already_exists" || code === "conflict")
      throw new Error("Пользователь с таким именем уже существует");
    if (msg) throw new Error(msg);
    throw new Error(`register failed: ${res.status}`);
  }
}

// --- Авторизованные запросы ---

/**
 * Fetch к main-service с авто-обновлением токена при 401.
 * При session_compromised бросает SessionCompromisedError — caller должен
 * вызвать clearAllTokens() и перейти на экран Login.
 */
export async function fetchAuth(
  path: string,
  opts: RequestInit = {}
): Promise<Response> {
  const access = await getAccessToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(opts.headers as Record<string, string>),
  };
  if (access) headers["Authorization"] = `Bearer ${access}`;

  let res = await fetch(`${apiUrl()}${path}`, { ...opts, headers });

  if (res.status === 401) {
    // Пробуем refresh (требует биометрию)
    const rt = await getRefreshToken("Подтвердите личность для обновления сессии");
    if (!rt) throw new SessionRevokedError();

    let newPair: TokenPair;
    try {
      newPair = await apiRefresh(rt);
    } catch (err) {
      if (
        err instanceof SessionCompromisedError ||
        err instanceof SessionExpiredError ||
        err instanceof SessionRevokedError
      ) {
        await clearAllTokens();
        throw err;
      }
      throw err;
    }

    await saveTokens({
      accessToken: newPair.access_token,
      refreshToken: newPair.refresh_token,
      sessionId: newPair.session_id,
    });

    headers["Authorization"] = `Bearer ${newPair.access_token}`;
    res = await fetch(`${apiUrl()}${path}`, { ...opts, headers });
  }

  return res;
}

// --- Push API ---

/** Возвращает VAPID public key для web push подписки. */
export async function getVapidPublicKey(): Promise<string> {
  const res = await fetch(`${apiUrl()}/api/v1/push/vapid-public-key`);
  if (!res.ok) throw new Error(`vapid-public-key: ${res.status}`);
  const data = (await res.json()) as { public_key: string };
  return data.public_key;
}

/** Регистрирует устройство для push-уведомлений. */
export async function registerDevice(
  platform: string,
  pushSubscription: unknown
): Promise<void> {
  const body =
    platform === "web"
      ? { platform, push_subscription: pushSubscription, device_id: getOrCreateDeviceId() }
      : { platform, device_id: getOrCreateDeviceId() };

  const res = await fetchAuth("/api/v1/devices/register", {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`registerDevice: ${res.status}`);
}

// --- Прикладные вызовы ---

export async function getUnreadCount(): Promise<number> {
  const res = await fetchAuth("/api/v1/me/unread-count");
  if (!res.ok) throw new Error(`unread-count: ${res.status}`);
  const data = (await res.json()) as { unread_count: number };
  return data.unread_count;
}

// --- Chat API ---

export interface Message {
  id: string;
  dialog_id: string;
  sender_id: string;
  body: string;
  created_at: string;
  expires_at: string | null;
}

export async function getMessages(
  dialogId: string,
  limit = 50,
  before?: string,
): Promise<Message[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (before) params.set("before", before);
  const res = await fetchAuth(`/api/v1/dialogs/${dialogId}/messages?${params}`);
  if (!res.ok) throw new Error(`getMessages: ${res.status}`);
  const data = (await res.json()) as { items: Message[]; next_before?: string };
  return data.items;
}

export async function sendMessage(dialogId: string, body: string): Promise<Message> {
  const res = await fetchAuth(`/api/v1/dialogs/${dialogId}/messages`, {
    method: "POST",
    body: JSON.stringify({ body }),
  });
  if (!res.ok) throw new Error(`sendMessage: ${res.status}`);
  const data = (await res.json()) as { message: Message };
  return data.message;
}

export async function markRead(messageId: string): Promise<void> {
  await fetchAuth(`/api/v1/messages/${messageId}/read`, { method: "POST" });
}
