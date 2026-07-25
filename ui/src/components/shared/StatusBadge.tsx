export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const colorMap: Record<string, string> = {
    online: 'bg-green-100 text-green-800',
    offline: 'bg-red-100 text-red-800',
    active: 'bg-green-100 text-green-800',
    inactive: 'bg-slate-100 text-slate-600',
    success: 'bg-green-100 text-green-800',
    failed: 'bg-red-100 text-red-800',
    firing: 'bg-red-100 text-red-800',
    resolved: 'bg-green-100 text-green-800',
    acknowledged: 'bg-blue-100 text-blue-800',
    pending: 'bg-slate-100 text-slate-600',
    running: 'bg-blue-100 text-blue-800',
    deploying: 'bg-blue-100 text-blue-800',
    completed: 'bg-green-100 text-green-800',
    timeout: 'bg-amber-100 text-amber-800',
    critical: 'bg-red-100 text-red-800',
    warning: 'bg-amber-100 text-amber-800',
    info: 'bg-blue-100 text-blue-800',
    open: 'bg-red-100 text-red-800',
    patched: 'bg-green-100 text-green-800',
    ignored: 'bg-slate-100 text-slate-600',
  };

  const dotMap: Record<string, string> = {
    online: 'bg-green-500',
    offline: 'bg-red-500',
    active: 'bg-green-500',
    pending: 'bg-slate-400',
    running: 'bg-blue-500',
    deploying: 'bg-blue-500',
    critical: 'bg-red-500',
    warning: 'bg-amber-500',
  };

  const cls = colorMap[status.toLowerCase()] || 'bg-slate-100 text-slate-600';
  const dotCls = dotMap[status.toLowerCase()];

  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${cls}`}>
      {dotCls && <span className={`w-1.5 h-1.5 rounded-full ${dotCls}`} />}
      {label || status}
    </span>
  );
}
