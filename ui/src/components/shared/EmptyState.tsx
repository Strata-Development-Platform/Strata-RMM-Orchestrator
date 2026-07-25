export function EmptyState({ icon = '📭', title, description, action }: {
  icon?: string;
  title: string;
  description?: string;
  action?: { label: string; onClick: () => void };
}) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <span className="text-4xl mb-4">{icon}</span>
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
