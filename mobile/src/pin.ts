/**
 * pin.ts — локальный PIN verifier + (Should) encrypt refresh.
 *
 * PIN никогда не уходит на сервер. В Preferences только salt + hash.
 * Схема: SHA-256(salt || PIN) для verify; PBKDF2(PIN, salt) → AES-GCM для refresh.
 */

import { Preferences } from "@capacitor/preferences";

export const PIN_LENGTH = 4;
export const PIN_MAX_ATTEMPTS = 5;
export const PIN_LOCK_GRACE_MS = 60_000;
export const PBKDF2_ITERATIONS = 100_000;

const KEY_PIN_SALT = "my_chat_pin_salt";
const KEY_PIN_HASH = "my_chat_pin_hash";
const KEY_PIN_SET = "my_chat_pin_set";

/** Prefix for encrypted refresh blob in Preferences. */
export const REFRESH_ENC_PREFIX = "enc:v1:";

const te = new TextEncoder();

export function isValidPinFormat(pin: string): boolean {
  return new RegExp(`^\\d{${PIN_LENGTH}}$`).test(pin);
}

export async function generateSalt(bytes = 16): Promise<Uint8Array> {
  const salt = new Uint8Array(bytes);
  crypto.getRandomValues(salt);
  return salt;
}

/** Verifier: SHA-256(salt || PIN). */
export async function hashPin(pin: string, salt: Uint8Array): Promise<Uint8Array> {
  const pinBytes = te.encode(pin);
  const data = new Uint8Array(salt.length + pinBytes.length);
  data.set(salt, 0);
  data.set(pinBytes, salt.length);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return new Uint8Array(digest);
}

export async function pinsMatch(
  pin: string,
  salt: Uint8Array,
  expectedHash: Uint8Array
): Promise<boolean> {
  const actual = await hashPin(pin, salt);
  if (actual.length !== expectedHash.length) return false;
  let diff = 0;
  for (let i = 0; i < actual.length; i++) {
    diff |= actual[i]! ^ expectedHash[i]!;
  }
  return diff === 0;
}

async function deriveAesKey(pin: string, salt: Uint8Array): Promise<CryptoKey> {
  const baseKey = await crypto.subtle.importKey(
    "raw",
    te.encode(pin),
    "PBKDF2",
    false,
    ["deriveKey"]
  );
  return crypto.subtle.deriveKey(
    {
      name: "PBKDF2",
      salt,
      iterations: PBKDF2_ITERATIONS,
      hash: "SHA-256",
    },
    baseKey,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"]
  );
}

/** AES-GCM ciphertext as base64(nonce || ciphertext). */
export async function encryptWithPin(
  pin: string,
  salt: Uint8Array,
  plaintext: string
): Promise<string> {
  const key = await deriveAesKey(pin, salt);
  const nonce = new Uint8Array(12);
  crypto.getRandomValues(nonce);
  const cipherBuf = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv: nonce },
    key,
    te.encode(plaintext)
  );
  const cipher = new Uint8Array(cipherBuf);
  const out = new Uint8Array(nonce.length + cipher.length);
  out.set(nonce, 0);
  out.set(cipher, nonce.length);
  return bytesToBase64(out);
}

export async function decryptWithPin(
  pin: string,
  salt: Uint8Array,
  ciphertextB64: string
): Promise<string> {
  const raw = base64ToBytes(ciphertextB64);
  if (raw.length < 13) {
    throw new Error("invalid ciphertext");
  }
  const nonce = raw.slice(0, 12);
  const cipher = raw.slice(12);
  const key = await deriveAesKey(pin, salt);
  const plainBuf = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv: nonce },
    key,
    cipher
  );
  return new TextDecoder().decode(plainBuf);
}

export function wrapEncryptedRefresh(ciphertextB64: string): string {
  return REFRESH_ENC_PREFIX + ciphertextB64;
}

export function unwrapEncryptedRefresh(stored: string): string | null {
  if (!stored.startsWith(REFRESH_ENC_PREFIX)) return null;
  return stored.slice(REFRESH_ENC_PREFIX.length);
}

export function isEncryptedRefresh(stored: string): boolean {
  return stored.startsWith(REFRESH_ENC_PREFIX);
}

export async function isPinSet(): Promise<boolean> {
  const [{ value: flag }, { value: salt }, { value: hash }] = await Promise.all([
    Preferences.get({ key: KEY_PIN_SET }),
    Preferences.get({ key: KEY_PIN_SALT }),
    Preferences.get({ key: KEY_PIN_HASH }),
  ]);
  if (flag === "1" && salt && hash) return true;
  return !!(salt && hash);
}

async function loadSaltAndHash(): Promise<{
  salt: Uint8Array;
  hash: Uint8Array;
} | null> {
  const [{ value: saltB64 }, { value: hashB64 }] = await Promise.all([
    Preferences.get({ key: KEY_PIN_SALT }),
    Preferences.get({ key: KEY_PIN_HASH }),
  ]);
  if (!saltB64 || !hashB64) return null;
  return { salt: base64ToBytes(saltB64), hash: base64ToBytes(hashB64) };
}

/** Создаёт PIN verifier. Не сохраняет plaintext. */
export async function setupPin(pin: string): Promise<void> {
  if (!isValidPinFormat(pin)) {
    throw new Error(`PIN must be exactly ${PIN_LENGTH} digits`);
  }
  const salt = await generateSalt();
  const hash = await hashPin(pin, salt);
  await Promise.all([
    Preferences.set({ key: KEY_PIN_SALT, value: bytesToBase64(salt) }),
    Preferences.set({ key: KEY_PIN_HASH, value: bytesToBase64(hash) }),
    Preferences.set({ key: KEY_PIN_SET, value: "1" }),
  ]);
}

export async function verifyPin(pin: string): Promise<boolean> {
  if (!isValidPinFormat(pin)) return false;
  const stored = await loadSaltAndHash();
  if (!stored) return false;
  return pinsMatch(pin, stored.salt, stored.hash);
}

/**
 * Смена PIN: verify old → write new salt/hash.
 * Если передан refreshToken — перешифровывает и возвращает новый blob для Preferences.
 */
export async function changePin(
  oldPin: string,
  newPin: string,
  encryptedRefresh?: string | null
): Promise<{ ok: true; refreshBlob?: string } | { ok: false; reason: "invalid_old" | "invalid_new" }> {
  if (!isValidPinFormat(newPin)) {
    return { ok: false, reason: "invalid_new" };
  }
  const ok = await verifyPin(oldPin);
  if (!ok) return { ok: false, reason: "invalid_old" };

  let plainRefresh: string | undefined;
  if (encryptedRefresh) {
    const stored = await loadSaltAndHash();
    if (!stored) return { ok: false, reason: "invalid_old" };
    const inner = unwrapEncryptedRefresh(encryptedRefresh) ?? encryptedRefresh;
    try {
      plainRefresh = isEncryptedRefresh(encryptedRefresh)
        ? await decryptWithPin(oldPin, stored.salt, inner)
        : encryptedRefresh;
    } catch {
      return { ok: false, reason: "invalid_old" };
    }
  }

  await setupPin(newPin);

  if (plainRefresh !== undefined) {
    const stored = await loadSaltAndHash();
    if (!stored) return { ok: false, reason: "invalid_new" };
    const cipher = await encryptWithPin(newPin, stored.salt, plainRefresh);
    return { ok: true, refreshBlob: wrapEncryptedRefresh(cipher) };
  }
  return { ok: true };
}

export async function clearPin(): Promise<void> {
  await Promise.all([
    Preferences.remove({ key: KEY_PIN_SALT }),
    Preferences.remove({ key: KEY_PIN_HASH }),
    Preferences.remove({ key: KEY_PIN_SET }),
  ]);
}

/** Encrypt refresh with current device salt + PIN. Requires PIN already set. */
export async function encryptRefreshToken(
  pin: string,
  refreshToken: string
): Promise<string> {
  const stored = await loadSaltAndHash();
  if (!stored) {
    throw new Error("PIN not set");
  }
  const cipher = await encryptWithPin(pin, stored.salt, refreshToken);
  return wrapEncryptedRefresh(cipher);
}

/** Decrypt refresh blob using PIN + stored salt. */
export async function decryptRefreshToken(
  pin: string,
  storedRefresh: string
): Promise<string> {
  const stored = await loadSaltAndHash();
  if (!stored) {
    throw new Error("PIN not set");
  }
  const inner = unwrapEncryptedRefresh(storedRefresh);
  if (!inner) {
    // plaintext legacy (migration) — return as-is
    return storedRefresh;
  }
  return decryptWithPin(pin, stored.salt, inner);
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]!);
  }
  return btoa(binary);
}

function base64ToBytes(b64: string): Uint8Array {
  const binary = atob(b64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    out[i] = binary.charCodeAt(i);
  }
  return out;
}
