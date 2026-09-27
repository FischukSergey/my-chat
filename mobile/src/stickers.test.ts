import { describe, expect, it } from "vitest";
import {
  STICKER_PACK,
  STICKER_PREVIEW_LABEL,
  encodeStickerBody,
  findSticker,
  formatMessagePreview,
  parseStickerBody,
  stickerAssetUrl,
} from "./stickers";

describe("parseStickerBody", () => {
  it("принимает валидный код", () => {
    expect(parseStickerBody("sticker:default/wave")).toEqual({
      pack: "default",
      id: "wave",
    });
  });

  it("отклоняет чужой префикс и регистр", () => {
    expect(parseStickerBody("STICKER:default/wave")).toBeNull();
    expect(parseStickerBody("смотри sticker:default/wave")).toBeNull();
    expect(parseStickerBody("sticker:default/wave ")).toBeNull();
  });

  it("отклоняет инъекцию пути", () => {
    expect(parseStickerBody("sticker:../x")).toBeNull();
    expect(parseStickerBody("sticker:default/../wave")).toBeNull();
    expect(parseStickerBody("sticker:default/wave.png")).toBeNull();
    expect(parseStickerBody("sticker:default/wa ve")).toBeNull();
  });
});

describe("formatMessagePreview", () => {
  it("подменяет стикер на подпись", () => {
    expect(formatMessagePreview("sticker:default/joy")).toBe(STICKER_PREVIEW_LABEL);
    expect(formatMessagePreview("  sticker:other/x  ")).toBe(STICKER_PREVIEW_LABEL);
  });

  it("оставляет обычный текст", () => {
    expect(formatMessagePreview("привет")).toBe("привет");
    expect(formatMessagePreview("")).toBe("");
  });
});

describe("catalog", () => {
  it("encode совпадает с парсером", () => {
    expect(parseStickerBody(encodeStickerBody(STICKER_PACK, "smile"))).toEqual({
      pack: STICKER_PACK,
      id: "smile",
    });
  });

  it("известный id даёт src из каталога", () => {
    expect(findSticker("default", "smile")?.src).toBe("/stickers/default/smile.png");
    expect(stickerAssetUrl({ pack: "default", id: "smile" })).toBe(
      "/stickers/default/smile.png",
    );
  });

  it("неизвестный валидный код не ломает url", () => {
    expect(findSticker("default", "unknown")).toBeUndefined();
    expect(stickerAssetUrl({ pack: "default", id: "unknown" })).toBe(
      "/stickers/default/unknown.png",
    );
  });
});
