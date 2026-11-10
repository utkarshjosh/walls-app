import { useEffect, useState, type FormEvent } from "react";
import { Button } from "./ui/button";
import { Dialog, DialogActions, DialogContent } from "./ui/dialog";
import { Input } from "./ui/form";

/** Ask for one name (new collection, rename). `onSubmit` returns an error sentence or null. */
export function NameDialog({
  open,
  title,
  initial = "",
  actionLabel,
  onSubmit,
  onClose,
}: {
  open: boolean;
  title: string;
  initial?: string;
  actionLabel: string;
  onSubmit: (name: string) => string | null;
  onClose: () => void;
}) {
  const [name, setName] = useState(initial);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName(initial);
      setError(null);
    }
  }, [open, initial]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const problem = onSubmit(name);
    if (problem) setError(problem);
    else onClose();
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent title={title} className="w-[min(380px,calc(100vw-32px))]">
        <form onSubmit={submit}>
          <Input
            autoFocus
            value={name}
            maxLength={60}
            placeholder="Collection name"
            aria-label="Collection name"
            aria-invalid={Boolean(error)}
            aria-describedby={error ? "name-error" : undefined}
            onChange={(event) => {
              setName(event.target.value);
              setError(null);
            }}
            onFocus={(event) => event.target.select()}
          />
          {error && (
            <p id="name-error" className="mt-2 text-sm text-danger" role="alert">
              {error}
            </p>
          )}
          <DialogActions>
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" type="submit" disabled={!name.trim()}>
              {actionLabel}
            </Button>
          </DialogActions>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Confirm something that cannot be undone. The button says exactly what will happen. */
export function ConfirmDialog({
  open,
  title,
  description,
  actionLabel,
  onConfirm,
  onClose,
}: {
  open: boolean;
  title: string;
  description: string;
  actionLabel: string;
  onConfirm: () => void;
  onClose: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent
        title={title}
        description={description}
        className="w-[min(400px,calc(100vw-32px))]"
      >
        <DialogActions className="mt-1">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="danger"
            onClick={() => {
              onConfirm();
              onClose();
            }}
          >
            {actionLabel}
          </Button>
        </DialogActions>
      </DialogContent>
    </Dialog>
  );
}
