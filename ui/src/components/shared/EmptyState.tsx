import { Inbox, type LucideIcon } from 'lucide-react';

export function EmptyState({ icon: Icon = Inbox, title, description, action }: {
  icon?: LucideIcon;
  title: string;
  description?: string;
  action?: { label: string; onClick: () => void };
}) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <div className="mb-4 rounded-xl bg-slate-100 p-3 text-slate-500 dark:bg-slate-800 dark:text-slate-400">
        <Icon size={28} strokeWidth={1.75} />
      </div>
      <h3 className="text-lg font-medium text-slate-900 dark:text-white mb-1">{title}</h3>
      {description && <p className="text-sm text-slate-500 mb-4">{description}</p>}
      {action && (
        <button onClick={action.onClick} className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700">
          {action.label}
        </button>
      )}
    </div>
  );
}
