import { openDB, type IDBPDatabase } from "idb";

// One local database for the whole app.
//  - "state": the library (collections, wallpapers, settings). Present since v1.
//  - "files": image/video blobs, used only when running in a browser tab;
//             the Mac app keeps files on disk instead.
let handle: Promise<IDBPDatabase> | undefined;

export function localDb(): Promise<IDBPDatabase> {
  handle ??= openDB("walls-local", 2, {
    upgrade(db) {
      if (!db.objectStoreNames.contains("state")) db.createObjectStore("state");
      if (!db.objectStoreNames.contains("files")) db.createObjectStore("files");
    },
  });
  // A failed open must not be cached forever (private windows can refuse storage).
  handle.catch(() => {
    handle = undefined;
  });
  return handle;
}
