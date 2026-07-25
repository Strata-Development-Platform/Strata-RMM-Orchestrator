export function Skeleton({ type = 'table', rows = 5, count = 4 }: { type?: 'table' | 'card' | 'text'; rows?: number; count?: number }) {
  if (type === 'card') {
    return (
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {Array.from({ length: count }).map((_, i) => (
          <div key={i} className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4 animate-pulse">
            <div className="h-3 bg-slate-200 dark:bg-slate-700 rounded w-16 mb-3" />
            <div className="h-8 bg-slate-200 dark:bg-slate-700 rounded w-12" />
          </div>
        ))}
      </div>
    );
  }

  if (type === 'text') {
    return (
      <div className="space-y-3 animate-pulse">
        {Array.from({ length: count }).map((_, i) => (
          <div key={i} className="h-4 bg-slate-200 dark:bg-slate-700 rounded" style={{ width: `${60 + Math.random() * 40}%` }} />
        ))}
      </div>
    );
  }

  return (
    <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden animate-pulse">
      <div className="bg-slate-50 dark:bg-slate-800 px-4 py-3 flex gap-8">
        {Array.from({ length: count }).map((_, i) => (
          <div key={i} className="h-3 bg-slate-200 dark:bg-slate-700 rounded w-20" />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="px-4 py-3 flex gap-8 border-t border-slate-100 dark:border-slate-800">
          {Array.from({ length: count }).map((_, j) => (
            <div key={j} className="h-3 bg-slate-100 dark:bg-slate-800 rounded" style={{ width: `${40 + Math.random() * 40}%` }} />
          ))}
        </div>
      ))}
    </div>
  );
}
