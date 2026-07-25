export function ConfirmDialog({ open, title, message, confirmLabel = 'Delete', onConfirm, onCancel }: {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  if (!open) return null;

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onCancel}>
      <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-sm w-full mx-4 p-6" onClick={e => e.stopPropagation()}>
        <h3 className="text-lg font-bold text-slate-900 dark:text-white mb-2">{title}</h3>
        <p className="text-sm text-slate-500 mb-6">{message}</p>
        <div className="flex justify-end gap-3">
          <button onClick={onCancel} className="px-4 py-2 text-sm rounded-md border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800">
            Cancel
          </button>
          <button onClick={onConfirm} className="px-4 py-2 text-sm rounded-md bg-red-600 text-white hover:bg-red-700">
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
