// Client for the catalog service (apps/api) that feeds Discover.
// Response shapes mirror apps/api exactly.

import type { WallpaperKind } from "@/platform/types";

export interface CatalogItem {
  id: string;
  title: string;
  kind: WallpaperKind;
  width: number;
  height: number;
  bytes: number;
  category: string;
  tags: string[];
  author: string;
  license: string;
  sourceUrl: string | null;
  color: string;
  createdAt: string;
  files: { thumb: string; preview: string; original: string; poster: string | null };
}

export interface CatalogCategory {
  slug: string;
  name: string;
  count: number;
}

export interface CatalogPack {
  id: string;
  name: string;
  description: string;
  count: number;
  covers: string[];
  items?: CatalogItem[];
}

export interface CatalogQuery {
  kind?: "image" | "live";
  category?: string;
  q?: string;
  wide?: boolean;
  cursor?: string;
  limit?: number;
}

export const API_BASE: string = (
  import.meta.env.VITE_API_URL ?? "http://localhost:8787"
).replace(/\/$/, "");

/** File URLs come back relative unless the server is configured with a public URL. */
export function fileUrl(path: string): string {
  return /^https?:\/\//.test(path) ? path : `${API_BASE}${path}`;
}

export class CatalogError extends Error {
  constructor(
    message: string,
    readonly offline: boolean,
  ) {
    super(message);
  }
}

async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, { signal, headers: { accept: "application/json" } });
  } catch (cause) {
    if ((cause as Error).name === "AbortError") throw cause;
    throw new CatalogError("Discover can't be reached right now.", true);
  }
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as { message?: string } | null;
    throw new CatalogError(body?.message ?? "Discover returned an unexpected answer.", false);
  }
  return (await response.json()) as T;
}

export const catalog = {
  categories: (signal?: AbortSignal) =>
    get<{ categories: CatalogCategory[] }>("/v1/categories", signal).then((r) => r.categories),

  wallpapers(query: CatalogQuery, signal?: AbortSignal) {
    const params = new URLSearchParams();
    if (query.kind) params.set("kind", query.kind);
    if (query.category) params.set("category", query.category);
    if (query.q) params.set("q", query.q);
    if (query.wide) params.set("wide", "1");
    if (query.cursor) params.set("cursor", query.cursor);
    params.set("limit", String(query.limit ?? 30));
    return get<{ items: CatalogItem[]; nextCursor: string | null }>(
      `/v1/wallpapers?${params}`,
      signal,
    );
  },

  packs: (signal?: AbortSignal) =>
    get<{ packs: CatalogPack[] }>("/v1/packs", signal).then((r) => r.packs),

  pack: (id: string, signal?: AbortSignal) =>
    get<CatalogPack & { items: CatalogItem[] }>(`/v1/packs/${encodeURIComponent(id)}`, signal),
};
