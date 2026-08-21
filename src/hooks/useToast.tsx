import React, { createContext, useCallback, useContext, useRef, useState } from 'react';
import type { ToastMessage, ToastType } from './toastTypes';
import { X, CheckCircle, AlertTriangle, AlertCircle, Info } from 'lucide-react';

const ToastContext = createContext<{
  addToast: (type: ToastType, message: string) => void;
} | null>(null);

type TimerEntry = { tid: ReturnType<typeof setTimeout>; startedAt: number; remaining: number };

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  const timersRef = useRef<Map<string, TimerEntry>>(new Map());

  const removeToast = useCallback((id: string) => {
    const entry = timersRef.current.get(id);
    if (entry) {
      clearTimeout(entry.tid);
      timersRef.current.delete(id);
    }
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const addToast = useCallback((type: ToastType, message: string) => {
    const id = Math.random().toString(36).slice(2);
    setToasts((prev) => [...prev, { id, type, message }]);
    const startedAt = Date.now();
    const tid = setTimeout(() => {
      timersRef.current.delete(id);
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 4000);
    timersRef.current.set(id, { tid, startedAt, remaining: 4000 });
  }, []);

  const pauseToast = useCallback((id: string) => {
    const entry = timersRef.current.get(id);
    if (!entry) return;
    clearTimeout(entry.tid);
    const elapsed = Date.now() - entry.startedAt;
    const remaining = Math.max(0, entry.remaining - elapsed);
    timersRef.current.set(id, { ...entry, remaining });
  }, []);

  const resumeToast = useCallback((id: string) => {
    const entry = timersRef.current.get(id);
    if (!entry) return;
    const startedAt = Date.now();
    const tid = setTimeout(() => {
      timersRef.current.delete(id);
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, entry.remaining);
    timersRef.current.set(id, { tid, startedAt, remaining: entry.remaining });
  }, []);

  const iconMap = {
    success: <CheckCircle className="w-5 h-5 text-[var(--color-wi-green)]" aria-hidden="true" />,
    warning: <AlertTriangle className="w-5 h-5 text-[var(--color-wi-yellow)]" aria-hidden="true" />,
    error: <AlertCircle className="w-5 h-5 text-[var(--color-wi-red)]" aria-hidden="true" />,
    info: <Info className="w-5 h-5 text-[var(--color-wi-primary)]" aria-hidden="true" />,
  };

  const bgMap = {
    success: 'border-l-4 border-[var(--color-wi-green)]',
    warning: 'border-l-4 border-[var(--color-wi-yellow)]',
    error: 'border-l-4 border-[var(--color-wi-red)]',
    info: 'border-l-4 border-[var(--color-wi-primary)]',
  };

  return (
    <ToastContext.Provider value={{ addToast }}>
      {children}
      <div className="fixed top-4 right-4 z-[9999] space-y-2 w-80" role="region" aria-live="polite">
        {toasts.map((toast) => {
          const role = toast.type === "warning" || toast.type === "error" ? "alert" : "status";
          return (
            <div
              key={toast.id}
              role={role}
              className={`${bgMap[toast.type]} bg-white shadow-md p-3 flex items-start gap-2 text-sm animate-toast-enter`}
              onMouseEnter={() => pauseToast(toast.id)}
              onMouseLeave={() => resumeToast(toast.id)}
              onFocusCapture={() => pauseToast(toast.id)}
              onBlurCapture={() => resumeToast(toast.id)}
            >
              {iconMap[toast.type]}
              <p className="text-gray-800 flex-1">{toast.message}</p>
              <button type="button" aria-label={`Dismiss ${toast.message}`} onClick={() => removeToast(toast.id)} className="text-gray-400 hover:text-gray-600">
                <X className="w-4 h-4" aria-hidden="true" />
              </button>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within ToastProvider');
  return ctx;
}
