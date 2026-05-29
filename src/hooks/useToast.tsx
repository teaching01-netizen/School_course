import React, { createContext, useContext, useState, useCallback } from 'react';
import type { ToastMessage, ToastType } from '../types';
import { X, CheckCircle, AlertTriangle, AlertCircle, Info } from 'lucide-react';

const ToastContext = createContext<{
  addToast: (type: ToastType, message: string) => void;
} | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);

  const addToast = useCallback((type: ToastType, message: string) => {
    const id = Math.random().toString(36).slice(2);
    setToasts((prev) => [...prev, { id, type, message }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 4000);
  }, []);

  const removeToast = (id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  };

  const iconMap = {
    success: <CheckCircle className="w-5 h-5 text-[var(--color-wi-green)]" />,
    warning: <AlertTriangle className="w-5 h-5 text-[var(--color-wi-yellow)]" />,
    error: <AlertCircle className="w-5 h-5 text-[var(--color-wi-red)]" />,
    info: <Info className="w-5 h-5 text-[var(--color-wi-primary)]" />,
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
      <div className="fixed top-4 right-4 z-[9999] space-y-2 w-80" role="alert" aria-live="assertive" aria-atomic="true">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={`${bgMap[toast.type]} bg-white shadow-md p-3 flex items-start gap-2 text-sm animate-toast-enter`}
          >
            {iconMap[toast.type]}
            <p className="text-gray-800 flex-1">{toast.message}</p>
            <button onClick={() => removeToast(toast.id)} className="text-gray-400 hover:text-gray-600">
              <X className="w-4 h-4" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within ToastProvider');
  return ctx;
}
