import { beforeEach, describe, expect, it, vi } from "vitest";

const store = new Map<string, string>();

vi.mock("@capacitor/preferences", () => ({
  Preferences: {
    get: async ({ key }: { key: string }) => ({
      value: store.get(key) ?? null,
    }),
    set: async ({ key, value }: { key: string; value: string }) => {
      store.set(key, value);
    },
    remove: async ({ key }: { key: string }) => {
      store.delete(key);
    },
  },
}));

import {
  PIN_LENGTH,
  changePin,
  clearPin,
  decryptRefreshToken,
  decryptWithPin,
  encryptRefreshToken,
  encryptWithPin,
  generateSalt,
  hashPin,
  isEncryptedRefresh,
  isPinSet,
  isValidPinFormat,
  pinsMatch,
  setupPin,
  verifyPin,
  wrapEncryptedRefresh,
  setSessionPin,
  getSessionPin,
  clearSessionPin,
} from "./pin";

beforeEach(() => {
  store.clear();
  clearSessionPin();
});

describe("isValidPinFormat", () => {
  it(`accepts exactly ${PIN_LENGTH} digits`, () => {
    expect(isValidPinFormat("1234")).toBe(true);
  });

  it("rejects wrong length or non-digits", () => {
    expect(isValidPinFormat("123")).toBe(false);
    expect(isValidPinFormat("12345")).toBe(false);
    expect(isValidPinFormat("12ab")).toBe(false);
    expect(isValidPinFormat("")).toBe(false);
  });
});

describe("hashPin / pinsMatch", () => {
  it("same PIN+salt → same hash; wrong PIN fails", async () => {
    const salt = await generateSalt();
    const hash = await hashPin("4242", salt);
    expect(await pinsMatch("4242", salt, hash)).toBe(true);
    expect(await pinsMatch("0000", salt, hash)).toBe(false);
  });

  it("different salts produce different hashes", async () => {
    const a = await hashPin("4242", await generateSalt());
    const b = await hashPin("4242", await generateSalt());
    expect(Buffer.from(a).equals(Buffer.from(b))).toBe(false);
  });
});

describe("encryptWithPin / decryptWithPin round-trip", () => {
  it("decrypts what was encrypted", async () => {
    const salt = await generateSalt();
    const cipher = await encryptWithPin("1357", salt, "refresh-secret-token");
    const plain = await decryptWithPin("1357", salt, cipher);
    expect(plain).toBe("refresh-secret-token");
  });

  it("wrong PIN cannot decrypt", async () => {
    const salt = await generateSalt();
    const cipher = await encryptWithPin("1357", salt, "refresh-secret-token");
    await expect(decryptWithPin("9999", salt, cipher)).rejects.toThrow();
  });
});

describe("setupPin / verifyPin / changePin / clearPin", () => {
  it("setup stores verifier only (no plaintext PIN)", async () => {
    await setupPin("2468");
    expect(await isPinSet()).toBe(true);
    expect(store.get("my_chat_pin_set")).toBe("1");
    expect(store.has("my_chat_pin_salt")).toBe(true);
    expect(store.has("my_chat_pin_hash")).toBe(true);
    for (const v of store.values()) {
      expect(v).not.toBe("2468");
    }
    expect(await verifyPin("2468")).toBe(true);
    expect(await verifyPin("0000")).toBe(false);
  });

  it("rejects invalid PIN on setup", async () => {
    await expect(setupPin("12")).rejects.toThrow(/4 digits/);
  });

  it("changePin updates verifier", async () => {
    await setupPin("1111");
    const result = await changePin("1111", "2222");
    expect(result).toEqual({ ok: true });
    expect(await verifyPin("1111")).toBe(false);
    expect(await verifyPin("2222")).toBe(true);
  });

  it("changePin re-encrypts refresh blob", async () => {
    await setupPin("1111");
    const blob = await encryptRefreshToken("1111", "rt-abc");
    expect(isEncryptedRefresh(blob)).toBe(true);

    const result = await changePin("1111", "2222", blob);
    expect(result.ok).toBe(true);
    if (!result.ok || !result.refreshBlob) throw new Error("expected refreshBlob");
    expect(await decryptRefreshToken("2222", result.refreshBlob)).toBe("rt-abc");
    await expect(decryptRefreshToken("1111", result.refreshBlob)).rejects.toThrow();
  });

  it("clearPin removes keys", async () => {
    await setupPin("3333");
    await clearPin();
    expect(await isPinSet()).toBe(false);
    expect(await verifyPin("3333")).toBe(false);
  });
});

describe("encryptRefreshToken helpers", () => {
  it("wraps with enc:v1: prefix", async () => {
    await setupPin("5555");
    const blob = await encryptRefreshToken("5555", "plain-refresh");
    expect(blob.startsWith("enc:v1:")).toBe(true);
    expect(await decryptRefreshToken("5555", blob)).toBe("plain-refresh");
  });

  it("legacy plaintext refresh passes through decrypt", async () => {
    await setupPin("5555");
    expect(await decryptRefreshToken("5555", "legacy-plain")).toBe("legacy-plain");
  });

  it("wrapEncryptedRefresh marks ciphertext", () => {
    expect(isEncryptedRefresh(wrapEncryptedRefresh("abc"))).toBe(true);
    expect(isEncryptedRefresh("abc")).toBe(false);
  });
});

describe("session PIN (in-memory)", () => {
  it("set/get/clear", () => {
    expect(getSessionPin()).toBeNull();
    setSessionPin("1234");
    expect(getSessionPin()).toBe("1234");
    clearSessionPin();
    expect(getSessionPin()).toBeNull();
  });

  it("clearPin also clears session PIN", async () => {
    setSessionPin("1234");
    await clearPin();
    expect(getSessionPin()).toBeNull();
  });
});
