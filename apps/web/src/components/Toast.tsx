import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { AnimatePresence, motion } from "motion/react";
import { cn } from "@/lib/utils";

// Quiet confirmations and actionable errors. One at a time, never stacked,
// never for routine success the person can already see on screen.

interface ToastOptions {
  tone?: "plain" | "error";
  action?: { label: string; run: () => void };
}
interface Toast extends ToastOptions {
  id: number;
  message: string;
}

const ToastContext = createContext<(message: string, options?: ToastOptions) => void>(() => {});

export const useToast = () => useContext(ToastContext);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<Toast | null>(null);
  const counter = useRef(0);

  const notify = useCallback((message: string, options?: ToastOptions) => {
    setToast({ id: ++counter.current, message, ...options });
  }, []);

  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => setToast(null), toast.action ? 7000 : 3200);
    return () => clearTimeout(timer);
  }, [toast]);

  return (
    <ToastContext.Provider value={notify}>
      {children}
      <div
        className="pointer-events-none fixed inset-x-0 bottom-6 z-[60] flex justify-center px-4"
        role="status"
        aria-live="polite"
      >
        <AnimatePresence mode="wait">
          {toast && (
            <motion.div
              key={toast.id}
              initial={{ opacity: 0, y: 6 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0 }}
              className={cn(
                "pointer-events-auto flex max-w-[620px] items-center gap-3 rounded-full bg-raised py-2 pr-2 pl-4 text-sm shadow-pop",
                !toast.action && "pr-4",
                toast.tone === "error" && "text-danger",
              )}
            >
              <span>{toast.message}</span>
              {toast.action && (
                <button
                  type="button"
                  className="shrink-0 rounded-full bg-hover px-3 py-1 font-medium whitespace-nowrap text-fg hover:bg-line-strong"
                  onClick={() => {
                    toast.action?.run();
                    setToast(null);
                  }}
                >
                  {toast.action.label}
                </button>
              )}
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </ToastContext.Provider>
  );
}
