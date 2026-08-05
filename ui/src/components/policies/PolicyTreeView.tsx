import { useState } from 'react';
import { ChevronDown, ChevronRight, Copy, Edit2, Shield, ShieldOff } from 'lucide-react';
import type {
  PolicyCategory,
  PolicyField,
  PolicyLevel,
  PolicyTree,
  PolicyScope,
} from './PolicyTreeTypes';

type OnFieldToggle = (category: PolicyCategory, scope: PolicyScope, fieldName: string, value: string | number | boolean | null) => void;

interface PolicyTreeViewProps {
  tree: PolicyTree;
  activeCategory: PolicyCategory;
  onCategoryChange: (category: PolicyCategory) => void;
  onFieldToggle?: OnFieldToggle;
  readOnly?: boolean;
}

const CATEGORY_LABELS: Record<PolicyCategory, string> = {
  patch: 'Patch Management',
  alerting: 'Alerting Rules',
  monitoring: 'Monitoring Thresholds',
  software: 'Software Deployment',
  scripts: 'Script Execution',
  maintenance: 'Maintenance Windows',
};

const SCOPE_STYLES: Record<PolicyScope, { bg: string; text: string; border: string; icon: React.ReactNode }> = {
  platform: {
    bg: 'bg-slate-50 dark:bg-slate-800/50',
    text: 'text-slate-900 dark:text-slate-100',
    border: 'border-slate-300 dark:border-slate-600',
    icon: <Shield className="h-4 w-4" />,
  },
  msp: {
    bg: 'bg-blue-50 dark:bg-blue-900/20',
    text: 'text-blue-900 dark:text-blue-100',
    border: 'border-blue-300 dark:border-blue-700',
    icon: <Shield className="h-4 w-4" />,
  },
  client: {
    bg: 'bg-amber-50 dark:bg-amber-900/20',
    text: 'text-amber-900 dark:text-amber-100',
    border: 'border-amber-300 dark:border-amber-700',
    icon: <Shield className="h-4 w-4" />,
  },
  site: {
    bg: 'bg-green-50 dark:bg-green-900/20',
    text: 'text-green-900 dark:text-green-100',
    border: 'border-green-300 dark:border-green-700',
    icon: <Shield className="h-4 w-4" />,
  },
  device: {
    bg: 'bg-purple-50 dark:bg-purple-900/20',
    text: 'text-purple-900 dark:text-purple-100',
    border: 'border-purple-300 dark:border-purple-700',
    icon: <Shield className="h-4 w-4" />,
  },
};

const SCOPE_ORDER: PolicyScope[] = ['platform', 'msp', 'client', 'site', 'device'];

// Resolved effective policy for a given category — merges all levels.
export function resolveEffectivePolicy(
  tree: PolicyTree,
  category: PolicyCategory
): Record<string, string | number | boolean | null> {
  const levels = tree.categories[category] || [];
  const resolved: Record<string, string | number | boolean | null> = {};
  for (const level of levels) {
    for (const field of level.fields) {
      resolved[field.name] = field.value;
    }
  }
  return resolved;
}

// Count of overridden fields across all levels for a category.
export function countOverrides(tree: PolicyTree, category: PolicyCategory): number {
  const levels = tree.categories[category] || [];
  return levels.reduce((count, level) => count + level.fields.filter((f) => f.overridden).length, 0);
}

// Get all fields that are overridden for a category.
export function getOverriddenFields(tree: PolicyTree, category: PolicyCategory): PolicyField[] {
  const levels = tree.categories[category] || [];
  return levels.flatMap((level) => level.fields.filter((f) => f.overridden));
}

function InheritanceBadge({ field }: { field: PolicyField }) {
  if (!field.overridden) return null;
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
      <Edit2 className="h-3 w-3" />
      Overrides {field.inheritedFrom || 'ancestor'}
    </span>
  );
}

function EffectiveValueBadge({ field }: { field: PolicyField }) {
  if (!field.overridden) return null;
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
      <Copy className="h-3 w-3" />
      Inherited: {String(field.inheritedFrom || '')}
    </span>
  );
}

function PolicyFieldRow({
  field,
  scope,
  onToggle,
  readOnly,
}: {
  field: PolicyField;
  scope: PolicyScope;
  onToggle?: OnFieldToggle;
  readOnly?: boolean;
}) {
  const styles = SCOPE_STYLES[scope];

  return (
    <div className={`flex items-center justify-between gap-4 border-l-2 pl-3 ${styles.border}`}>
      <div className="flex flex-col">
        <span className={`text-sm font-medium ${styles.text}`}>{field.name}</span>
        <span className="text-xs text-slate-500 dark:text-slate-400">
          {field.overridden ? (
            <>
              <span className="text-red-600 dark:text-red-400">Override: </span>
              <code className="rounded bg-slate-100 px-1 py-0.5 text-xs dark:bg-slate-700">
                {String(field.value)}
              </code>
            </>
          ) : (
            <>
              <span className="text-green-600 dark:text-green-400">Effective: </span>
              <code className="rounded bg-slate-100 px-1 py-0.5 text-xs dark:bg-slate-700">
                {String(field.value)}
              </code>
              {field.inheritedFrom && (
                <span className="ml-1 text-slate-400">(from {field.inheritedFrom})</span>
              )}
            </>
          )}
        </span>
      </div>
      <div className="flex items-center gap-2">
        <InheritanceBadge field={field} />
        <EffectiveValueBadge field={field} />
        {!readOnly && onToggle && (
          <button
            onClick={() => onToggle(field.overridden ? field.name : field.name, field.value)}
            className="rounded-md p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-700 dark:hover:text-slate-300"
            title={field.overridden ? 'Remove override' : 'Add override'}
          >
            {field.overridden ? <ShieldOff className="h-4 w-4" /> : <Edit2 className="h-4 w-4" />}
          </button>
        )}
      </div>
    </div>
  );
}

function PolicyLevelRow({
  level,
  onToggle,
  readOnly,
}: {
  level: PolicyLevel;
  onToggle?: OnFieldToggle;
  readOnly?: boolean;
}) {
  const [expanded, setExpanded] = useState(true);
  const styles = SCOPE_STYLES[level.scope];
  const overrideCount = level.fields.filter((f) => f.overridden).length;

  return (
    <div className={`${styles.bg} rounded-lg border ${styles.border} p-4`}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-2 text-left"
      >
        {expanded ? (
          <ChevronDown className="h-4 w-4 text-slate-400" />
        ) : (
          <ChevronRight className="h-4 w-4 text-slate-400" />
        )}
        <span className={`${styles.text} flex items-center gap-2 text-sm font-semibold`}>
          {styles.icon}
          {level.label}
        </span>
        {overrideCount > 0 && (
          <span className="rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/40 dark:text-red-300">
            {overrideCount} override{overrideCount > 1 ? 's' : ''}
          </span>
        )}
      </button>
      {expanded && (
        <div className="mt-3 space-y-2 pl-6">
          {level.fields.length > 0 ? (
            level.fields.map((field) => (
              <PolicyFieldRow
                key={field.name}
                field={field}
                scope={level.scope}
                onToggle={onToggle}
                readOnly={readOnly}
              />
            ))
          ) : (
            <div className="flex items-center gap-2 pl-2 text-xs text-slate-400 italic">
              <Shield className="h-3 w-3" />
              No overrides — inherits from nearest ancestor
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// Effective policy summary panel at the top.
function EffectivePolicyPanel({
  tree,
  category,
}: {
  tree: PolicyTree;
  category: PolicyCategory;
}) {
  const effective = resolveEffectivePolicy(tree, category);
  const overridden = countOverrides(tree, category);

  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm dark:border-slate-700 dark:bg-slate-800">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
          Effective Policy — {CATEGORY_LABELS[category]}
        </h3>
        <span className="text-xs text-slate-500 dark:text-slate-400">
          {overridden} override{overridden !== 1 ? 's' : ''} across all levels
        </span>
      </div>
      <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
        {Object.entries(effective).map(([key, value]) => (
          <div key={key} className="rounded-md bg-slate-50 px-3 py-2 dark:bg-slate-700/50">
            <div className="text-xs text-slate-500 dark:text-slate-400">{key}</div>
            <div className="text-sm font-mono font-medium text-slate-900 dark:text-slate-100">
              {String(value)}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

export default function PolicyTreeView({
  tree,
  activeCategory,
  onCategoryChange,
  onFieldToggle,
  readOnly = false,
}: PolicyTreeViewProps) {
  const categories = Object.keys(tree.categories) as PolicyCategory[];
  const levels = tree.categories[activeCategory] || [];

  return (
    <div className="space-y-4">
      {/* Category selector */}
      <div className="flex flex-wrap gap-2">
        {categories.map((cat) => {
          const isActive = cat === activeCategory;
          const overrideCount = countOverrides(tree, cat);
          return (
            <button
              key={cat}
              onClick={() => onCategoryChange(cat)}
              className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium transition-colors ${
                isActive
                  ? 'border-blue-300 bg-blue-50 text-blue-700 dark:border-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                  : 'border-slate-200 bg-white text-slate-600 hover:border-slate-300 hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400 dark:hover:border-slate-600 dark:hover:bg-slate-700'
              }`}
            >
              {CATEGORY_LABELS[cat]}
              {overrideCount > 0 && (
                <span className="rounded-full bg-red-100 px-1.5 py-0.5 text-xs text-red-700 dark:bg-red-900/40 dark:text-red-300">
                  {overrideCount}
                </span>
              )}
            </button>
          );
        })}
      </div>

      {/* Effective policy summary */}
      <EffectivePolicyPanel tree={tree} category={activeCategory} />

      {/* Hierarchy levels */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
          Inheritance Hierarchy
        </h3>
        <div className="relative space-y-3 pl-4">
          {/* Vertical connection line */}
          <div className="absolute left-[17px] top-0 h-full w-0.5 bg-slate-200 dark:bg-slate-700" />
          {SCOPE_ORDER.map((scope) => {
            const level = levels.find((l) => l.scope === scope);
            if (!level) return null;
            return <PolicyLevelRow key={scope} level={level} onToggle={onFieldToggle} readOnly={readOnly} />;
          })}
        </div>
      </div>
    </div>
  );
}
