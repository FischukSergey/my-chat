/**
 * main.ts — точка входа. Управляет состояниями экранов:
 *
 *   [login]  ← нет refresh token / session_compromised / logout
 *      ↓ после login
 *   [unlock] ← есть refresh token (cold start)
 *      ↓ биометрия + refresh
 *   [home]   ← есть актуальный access token
 */

import {
  BiometryErrorType,
  clearAllTokens,
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
  getUnreadCount,
  SessionCompromisedError,
  SessionExpiredError,
  SessionRevokedError,
} from "./api";

// --- DOM helpers ---

function el<T extends HTMLElement>(id: string): T {
  const e = document.getElementById(id);
  if (!e) throw new Error(`Element #${id} not found`);
  return e as T;
}

function showScreen(name: "login" | "unlock" | "home"): void {
  for (const s of ["login", "unlock", "home"] as const) {
    const div = el(s);
    if (s === name) {
      div.classList.add("active");
    } else {
      div.classList.remove("active");
    }
  }
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

// --- Экран: Home ---

async function loadHome(): Promise<void> {
  showScreen("home");
  setStatus("Загрузка...");
  try {
    const count = await getUnreadCount();
    el("unread-count").textContent = String(count);
    setStatus(`Загружено`);
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

  // Проверка biometric availability
  const biometricOk = await isBiometricAvailable();
  log(`biometric available: ${biometricOk}`);

  // Cold start routing
  const hasRefresh = await hasRefreshToken();
  log(`has refresh token: ${hasRefresh}`);

  if (hasRefresh) {
    await startUnlock();
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
