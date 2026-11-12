import { useEffect, useState, type FormEvent } from "react";
import { account, useAccount } from "@/api/account";
import { useToast } from "./Toast";
import { Button } from "./ui/button";
import { Dialog, DialogActions, DialogContent } from "./ui/dialog";
import { Input, Label } from "./ui/form";

/**
 * Signing in, and the account once signed in.
 *
 * Signing in is optional and takes two steps with no password: an address,
 * then the six-digit code that was emailed to it. What signing in means is
 * said before the address is asked for, not after.
 */
export function AccountDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const signedIn = useAccount();
  const notify = useToast();
  const [step, setStep] = useState<"email" | "code">("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  useEffect(() => {
    if (!open) return;
    setStep("email");
    setCode("");
    setError(null);
    setBusy(false);
    setConfirmingDelete(false);
  }, [open]);

  /** Runs one request, showing its failure in the dialog. Reports whether it worked. */
  const attempt = async (work: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await work();
      return true;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "That did not work. Try again in a moment.");
      return false;
    } finally {
      setBusy(false);
    }
  };

  const sendCode = async (event?: FormEvent) => {
    event?.preventDefault();
    if (await attempt(() => account.start(email.trim()))) {
      setCode("");
      setStep("code");
    }
  };

  const signIn = async (event: FormEvent) => {
    event.preventDefault();
    if (await attempt(() => account.verify(email.trim(), code))) {
      onClose();
      notify(`Signed in as ${email.trim().toLowerCase()}`);
    }
  };

  const signOut = async () => {
    if (await attempt(() => account.signOut())) {
      onClose();
      notify("Signed out");
    }
  };

  const remove = async () => {
    if (await attempt(() => account.remove())) {
      onClose();
      notify("Your account and everything recorded with it have been deleted");
    }
  };

  const problem = error && (
    <p id="account-error" className="mt-2 text-sm text-danger" role="alert">
      {error}
    </p>
  );

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      {signedIn ? (
        <DialogContent title="Account" description={signedIn.email}>
          {confirmingDelete ? (
            <>
              <p className="text-sm text-fg-muted">
                This deletes your account and everything recorded with it: what you looked at, saved, set and searched
                for. Your library on this Mac is not touched. It cannot be undone.
              </p>
              {problem}
              <DialogActions>
                <Button variant="ghost" onClick={() => setConfirmingDelete(false)} disabled={busy}>
                  Cancel
                </Button>
                <Button variant="danger" onClick={remove} disabled={busy}>
                  Delete account
                </Button>
              </DialogActions>
            </>
          ) : (
            <>
              <p className="text-sm text-fg-muted">
                While you are signed in, Still records which catalog wallpapers you open, save, set and remove, and
                what you search for in Discover, so it can suggest others later. Pictures and videos you added
                yourself are never reported.
              </p>
              {problem}
              <DialogActions className="justify-between">
                <Button variant="ghost" onClick={() => setConfirmingDelete(true)} disabled={busy}>
                  Delete account…
                </Button>
                <Button onClick={signOut} disabled={busy}>
                  Sign out
                </Button>
              </DialogActions>
            </>
          )}
        </DialogContent>
      ) : step === "email" ? (
        <DialogContent
          title="Sign in"
          description="Optional. Still works the same without it."
        >
          <form onSubmit={sendCode}>
            <p className="mb-4 text-sm text-fg-muted">
              Signing in lets Discover learn what you like. While you are signed in, Still records which catalog
              wallpapers you open, save, set and remove, and what you search for. Pictures and videos you added
              yourself are never reported. You can delete it all at any time.
            </p>
            <Label htmlFor="account-email">Email address</Label>
            <Input
              id="account-email"
              className="mt-1.5"
              type="email"
              autoFocus
              autoComplete="email"
              inputMode="email"
              maxLength={254}
              placeholder="you@example.com"
              value={email}
              aria-invalid={Boolean(error)}
              aria-describedby={error ? "account-error" : undefined}
              onChange={(event) => {
                setEmail(event.target.value);
                setError(null);
              }}
            />
            {problem}
            <DialogActions>
              <Button variant="ghost" onClick={onClose}>
                Not now
              </Button>
              <Button variant="primary" type="submit" disabled={busy || !email.trim()}>
                Email me a code
              </Button>
            </DialogActions>
          </form>
        </DialogContent>
      ) : (
        <DialogContent
          title="Enter the code"
          description={`We sent six digits to ${email.trim()}. There is no password.`}
        >
          <form onSubmit={signIn}>
            <Label htmlFor="account-code">Code</Label>
            <Input
              id="account-code"
              className="mt-1.5 text-base tracking-[0.3em] tabular-nums"
              autoFocus
              autoComplete="one-time-code"
              inputMode="numeric"
              maxLength={7}
              placeholder="000000"
              value={code}
              aria-invalid={Boolean(error)}
              aria-describedby={error ? "account-error" : undefined}
              onChange={(event) => {
                setCode(event.target.value.replace(/[^0-9 ]/g, ""));
                setError(null);
              }}
            />
            {problem}
            <DialogActions className="justify-between">
              <span className="flex gap-1">
                <Button variant="ghost" onClick={() => setStep("email")} disabled={busy}>
                  Change address
                </Button>
                <Button variant="ghost" onClick={() => sendCode()} disabled={busy}>
                  Send again
                </Button>
              </span>
              <Button variant="primary" type="submit" disabled={busy || code.replace(/\D/g, "").length !== 6}>
                Sign in
              </Button>
            </DialogActions>
          </form>
        </DialogContent>
      )}
    </Dialog>
  );
}
