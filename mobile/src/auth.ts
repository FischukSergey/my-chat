/**
 * auth.ts — secure storage wrapper + biometric gate.
 *
 * Инвариант: getRefreshToken() всегда требует биометрическую аутентификацию
 * перед чтением из хранилища. Сервер о биометрии не знает — защита только локальная.
 */

import { Preferences } from "@capacitor/preferences";
import {
  BiometricAuth,
  BiometryError,
  BiometryErrorType,
} from "@aparajita/capacitor-biometric-auth";

const KEY_REFRESH = "my_chat_refresh_token";
const KEY_ACCESS = "my_chat_access_token";
const KEY_SESSION = "my_chat_session_id";
const KEY_USER_ID = "my_chat_user_id";

// --- Biometric ---

export async function isBiometricAvailable(): Promise<boolean> {
  try {
    const result = await BiometricAuth.checkBiometry();
    return result.isAvailable;
  } catch {
    return false;
  }
}

/**
 * Запускает биометрический prompt.
 * Возвращает true при успехе.
 * Бросает BiometryError при отказе или смене биометрии.
 */
export async function authenticate(reason: string): Promise<void> {
  await BiometricAuth.authenticate({
    reason,
    cancelTitle: "Отмена",
    iosFallbackTitle: "Ввести пароль",
    androidTitle: "Биометрическая аутентификация",
    androidSubtitle: reason,
    allowDeviceCredential: true,
  });
}

export { BiometryError, BiometryErrorType };

// --- Secure storage ---

/** Сохраняет токены после login/refresh без биометрии (запись не требует auth). */
export async function saveTokens(tokens: {
  accessToken: string;
  refreshToken: string;
  sessionId: string;
  userId?: string;
}): Promise<void> {
  await Promise.all([
    Preferences.set({ key: KEY_ACCESS, value: tokens.accessToken }),
    Preferences.set({ key: KEY_REFRESH, value: tokens.refreshToken }),
    Preferences.set({ key: KEY_SESSION, value: tokens.sessionId }),
    tokens.userId
      ? Preferences.set({ key: KEY_USER_ID, value: tokens.userId })
      : Promise.resolve(),
  ]);
}

/** Читает access token без биометрии (он короткоживущий, не секрет). */
export async function getAccessToken(): Promise<string | null> {
  const { value } = await Preferences.get({ key: KEY_ACCESS });
  return value;
}

/** Читает refresh token — ТРЕБУЕТ биометрию. */
export async function getRefreshToken(
  reason = "Подтвердите личность для входа"
): Promise<string | null> {
  const { value } = await Preferences.get({ key: KEY_REFRESH });
  if (!value) return null;

  await authenticate(reason);
  return value;
}

/** Читает refresh token БЕЗ биометрии — только для проверки наличия. */
export async function hasRefreshToken(): Promise<boolean> {
  const { value } = await Preferences.get({ key: KEY_REFRESH });
  return !!value;
}

export async function getSessionId(): Promise<string | null> {
  const { value } = await Preferences.get({ key: KEY_SESSION });
  return value;
}

export async function getSavedUserId(): Promise<string | null> {
  const { value } = await Preferences.get({ key: KEY_USER_ID });
  return value;
}

/** Полная очистка — вызывается при logout и session_compromised. */
export async function clearAllTokens(): Promise<void> {
  await Promise.all([
    Preferences.remove({ key: KEY_REFRESH }),
    Preferences.remove({ key: KEY_ACCESS }),
    Preferences.remove({ key: KEY_SESSION }),
  ]);
}
