/** Встроенный пак стикеров (Sprint 11). Код в body: sticker:{pack}/{id}. */

export const STICKER_PACK = "default";

export const STICKER_BODY_RE = /^sticker:([a-z0-9_-]+)\/([a-z0-9_-]+)$/;

export const STICKER_PREVIEW_LABEL = "Стикер";

export type StickerRef = {
  pack: string;
  id: string;
};

export type StickerDef = StickerRef & {
  src: string;
  label: string;
};

/** Каталог пака default — id совпадают с файлами в /stickers/default/{id}.png */
export const STICKERS: readonly StickerDef[] = [
  { pack: STICKER_PACK, id: "smile", src: "/stickers/default/smile.png", label: "Улыбка" },
  { pack: STICKER_PACK, id: "happy", src: "/stickers/default/happy.png", label: "Радость" },
  { pack: STICKER_PACK, id: "joy", src: "/stickers/default/joy.png", label: "Смех" },
  { pack: STICKER_PACK, id: "love", src: "/stickers/default/love.png", label: "Влюблённость" },
  { pack: STICKER_PACK, id: "neutral", src: "/stickers/default/neutral.png", label: "Нейтрально" },
  { pack: STICKER_PACK, id: "sad", src: "/stickers/default/sad.png", label: "Грусть" },
  { pack: STICKER_PACK, id: "cry", src: "/stickers/default/cry.png", label: "Слёзы" },
  { pack: STICKER_PACK, id: "angry", src: "/stickers/default/angry.png", label: "Злость" },
  { pack: STICKER_PACK, id: "wow", src: "/stickers/default/wow.png", label: "Удивление" },
  { pack: STICKER_PACK, id: "think", src: "/stickers/default/think.png", label: "Раздумье" },
  { pack: STICKER_PACK, id: "heart", src: "/stickers/default/heart.png", label: "Сердце" },
  { pack: STICKER_PACK, id: "wave", src: "/stickers/default/wave.png", label: "Привет" },
  { pack: STICKER_PACK, id: "thumbsup", src: "/stickers/default/thumbsup.png", label: "Ок" },
  { pack: STICKER_PACK, id: "thanks", src: "/stickers/default/thanks.png", label: "Спасибо" },
  { pack: STICKER_PACK, id: "fire", src: "/stickers/default/fire.png", label: "Огонь" },
  { pack: STICKER_PACK, id: "party", src: "/stickers/default/party.png", label: "Праздник" },
] as const;

const STICKER_BY_KEY = new Map(STICKERS.map((s) => [`${s.pack}/${s.id}`, s]));

export function parseStickerBody(body: string): StickerRef | null {
  const m = STICKER_BODY_RE.exec(body);
  if (!m) return null;
  return { pack: m[1], id: m[2] };
}

export function encodeStickerBody(pack: string, id: string): string {
  return `sticker:${pack}/${id}`;
}

export function findSticker(pack: string, id: string): StickerDef | undefined {
  return STICKER_BY_KEY.get(`${pack}/${id}`);
}

export function stickerAssetUrl(ref: StickerRef): string {
  const known = findSticker(ref.pack, ref.id);
  if (known) return known.src;
  return `/stickers/${ref.pack}/${ref.id}.png`;
}

/** Home preview: стикер → «Стикер», иначе исходный текст. */
export function formatMessagePreview(text: string): string {
  const trimmed = text.trim();
  if (!trimmed) return trimmed;
  return parseStickerBody(trimmed) ? STICKER_PREVIEW_LABEL : text;
}
