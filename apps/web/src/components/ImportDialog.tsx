import { useEffect, useRef, useState, type DragEvent } from "react";
import { AlertCircle, Check, ImagePlus, X } from "lucide-react";
import { ACCEPT } from "@/lib/media";
import { cn, plural } from "@/lib/utils";
import { useLibrary, type ImportResult } from "@/state/store";
import { Button } from "./ui/button";
import { Dialog, DialogActions, DialogContent } from "./ui/dialog";
import { Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/form";

const NEW_COLLECTION = "__new__";

/**
 * Add pictures and videos from this computer.
 * (Shadcn Space "File Upload 01" reduced to its drop area and file list, wired
 * to the app's real checks. Each file reports its own result; one bad file
 * never loses the good ones.)
 */
export function ImportDialog({
  open,
  onClose,
  collectionId,
  initialFiles,
}: {
  open: boolean;
  onClose: () => void;
  /** Collection that is open behind the dialog, preselected as the destination. */
  collectionId: string | null;
  /** Files dropped on the window before the dialog opened. */
  initialFiles?: File[];
}) {
  const { state, importFiles, createCollection } = useLibrary();
  const [files, setFiles] = useState<File[]>([]);
  const [results, setResults] = useState<ImportResult[]>([]);
  const [destination, setDestination] = useState("");
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState(0);
  const [over, setOver] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setFiles(initialFiles ?? []);
    setResults([]);
    setProgress(0);
    setOver(false);
    setDestination(collectionId ?? state.collections[0]?.id ?? NEW_COLLECTION);
    // Reset only when the dialog opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const add = (list: FileList | File[] | null) => {
    if (!list) return;
    setFiles((current) => [...current, ...Array.from(list)]);
    setResults([]);
  };

  const onDrop = (event: DragEvent) => {
    event.preventDefault();
    setOver(false);
    if (!busy) add(event.dataTransfer.files);
  };

  async function submit() {
    if (!files.length || busy) return;
    setBusy(true);
    let target = destination;
    if (target === NEW_COLLECTION) {
      const base = "New collection";
      let name = base;
      for (let n = 2; state.collections.some((c) => c.name.toLowerCase() === name.toLowerCase()); n++)
        name = `${base} ${n}`;
      const created = createCollection(name);
      if ("error" in created) {
        setBusy(false);
        return;
      }
      target = created.id;
      setDestination(target);
    }
    const outcome = await importFiles(files, target, (done) => setProgress(done));
    setResults(outcome);
    setFiles([]);
    setBusy(false);
    // Everything went in: nothing left to read, so get out of the way.
    if (outcome.every((result) => result.ok)) onClose();
  }

  const failed = results.filter((result) => !result.ok);
  const added = results.length - failed.length;

  return (
    <Dialog open={open} onOpenChange={(next) => !next && !busy && onClose()}>
      <DialogContent title="Add wallpapers" className="w-[min(520px,calc(100vw-32px))]">
        <div
          onDragOver={(event) => {
            event.preventDefault();
            setOver(true);
          }}
          onDragLeave={() => setOver(false)}
          onDrop={onDrop}
          className={cn(
            "flex flex-col items-center rounded-panel border border-dashed border-line-strong px-6 py-8 text-center transition-colors duration-100",
            over && "border-tint bg-tint/8",
          )}
        >
          <ImagePlus className="size-6 text-fg-muted" aria-hidden />
          <p className="mt-3 font-medium">Drop pictures or videos here</p>
          <p className="mt-1 text-sm text-fg-muted">
            JPEG, PNG or WebP up to 25 MB. MP4, MOV or WebM up to 200 MB.
          </p>
          <Button className="mt-4" onClick={() => input.current?.click()} disabled={busy}>
            Choose files
          </Button>
          <input
            ref={input}
            type="file"
            accept={ACCEPT}
            multiple
            hidden
            onChange={(event) => {
              add(event.target.files);
              event.target.value = "";
            }}
          />
        </div>

        {files.length > 0 && (
          <ul className="scroll-area mt-3 max-h-40 space-y-1">
            {files.map((file, index) => (
              <li
                key={`${file.name}-${index}`}
                className="flex h-8 items-center gap-2 rounded-control bg-raised px-2.5 text-sm"
              >
                <span className="min-w-0 flex-1 truncate">{file.name}</span>
                <span className="shrink-0 text-xs text-fg-faint tabular-nums">
                  {(file.size / 1048576).toFixed(1)} MB
                </span>
                {!busy && (
                  <button
                    type="button"
                    aria-label={`Remove ${file.name}`}
                    className="rounded text-fg-faint hover:text-fg"
                    onClick={() => setFiles((current) => current.filter((_, i) => i !== index))}
                  >
                    <X className="size-3.5" />
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}

        {results.length > 0 && (
          <div className="mt-3" role="status">
            {added > 0 && (
              <p className="flex items-center gap-1.5 text-sm text-fg-muted">
                <Check className="size-4 text-tint" aria-hidden />
                {plural(added, "wallpaper")} added
              </p>
            )}
            {failed.length > 0 && (
              <ul className="scroll-area mt-2 max-h-40 space-y-1">
                {failed.map((result, index) => (
                  <li
                    key={`${result.name}-${index}`}
                    className="flex items-start gap-2 rounded-control bg-danger/10 px-2.5 py-2 text-sm"
                  >
                    <AlertCircle className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden />
                    <span className="min-w-0">
                      <span className="block truncate font-medium">{result.name}</span>
                      <span className="block text-fg-muted">{result.message}</span>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}

        <div className="mt-4 flex items-center gap-3">
          <Label htmlFor="import-destination" className="shrink-0">
            Add to
          </Label>
          <Select value={destination} onValueChange={setDestination} disabled={busy}>
            <SelectTrigger id="import-destination" className="min-w-0 flex-1">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {state.collections.map((collection) => (
                <SelectItem key={collection.id} value={collection.id}>
                  {collection.name}
                </SelectItem>
              ))}
              <SelectItem value={NEW_COLLECTION}>New collection</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <DialogActions>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {results.length ? "Done" : "Cancel"}
          </Button>
          <Button variant="primary" onClick={submit} disabled={!files.length || busy}>
            {busy
              ? `Adding ${Math.min(progress + 1, files.length)} of ${files.length}…`
              : files.length
                ? `Add ${plural(files.length, "file")}`
                : "Add"}
          </Button>
        </DialogActions>
      </DialogContent>
    </Dialog>
  );
}
