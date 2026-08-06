import { useState } from 'react';
import {
  Plus,
  Trash2,
  ChevronRight,
  ChevronDown,
  Settings2,
  Filter,
  Code,
} from 'lucide-react';
import { useToast } from '@/components/shared/Toast';
import type {
  SmartGroupExpression,
  SmartGroupFilter,
  Operator,
  LogicOperator,
  FieldOption,
} from './SmartGroupTypes';
import {
  FIELD_OPTIONS,
  OPERATOR_LABELS,
} from './SmartGroupTypes';

interface SmartGroupExpressionBuilderProps {
  initialExpression?: SmartGroupExpression | null;
  onExpressionChange: (expression: SmartGroupExpression) => void;
  readOnly?: boolean;
}

const LOGIC_OPTIONS: { label: string; value: LogicOperator }[] = [
  { label: 'AND (all conditions match)', value: 'AND' },
  { label: 'OR (any condition matches)', value: 'OR' },
];

export default function SmartGroupExpressionBuilder({
  initialExpression,
  onExpressionChange,
  readOnly = false,
}: SmartGroupExpressionBuilderProps) {
  const { showToast } = useToast();
  const [expression, setExpression] = useState<SmartGroupExpression>(
    initialExpression || { condition: 'AND', filters: [] }
  );
  const [expanded, setExpanded] = useState(true);
  const [viewMode, setViewMode] = useState<'visual' | 'json'>('visual');

  const addFilter = () => {
    const newFilter: SmartGroupFilter = {
      field: '',
      op: 'eq',
      value: '',
    };
    const updated = {
      ...expression,
      filters: [...(expression.filters || []), newFilter],
    };
    setExpression(updated);
    onExpressionChange(updated);
  };

  const removeFilter = (index: number) => {
    const updated = {
      ...expression,
      filters: (expression.filters || []).filter((_, i) => i !== index),
    };
    setExpression(updated);
    onExpressionChange(updated);
  };

  const updateFilter = (index: number, key: keyof SmartGroupFilter, value: string) => {
    const filters = [...(expression.filters || [])];
    filters[index] = { ...filters[index], [key]: value };
    const updated = { ...expression, filters };
    setExpression(updated);
    onExpressionChange(updated);
  };

  const addNestedGroup = () => {
    const newNested: SmartGroupExpression = {
      condition: 'AND',
      filters: [],
    };
    const updated = {
      ...expression,
      children: [...(expression.children || []), newNested],
    };
    setExpression(updated);
    onExpressionChange(updated);
  };

  const removeNested = (index: number) => {
    const updated = {
      ...expression,
      children: (expression.children || []).filter((_, i) => i !== index),
    };
    setExpression(updated);
    onExpressionChange(updated);
  };

  const updateLogicOperator = (op: LogicOperator) => {
    const updated = { ...expression, condition: op };
    setExpression(updated);
    onExpressionChange(updated);
  };

  const serializeExpression = () => {
    try {
      const json = JSON.stringify(expression, null, 2);
      showToast('info', 'Expression copied to clipboard', json);
    } catch {
      showToast('error', 'Failed to serialize expression');
    }
  };

  const expressionIsValid = (): boolean => {
    if (!expression.filters || expression.filters.length === 0) {
      return expression.children && expression.children.length > 0;
    }
    return expression.filters.every(
      (f) => f.field !== '' && f.op !== '' && f.value !== ''
    );
  };

  return (
    <div className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-slate-200 dark:border-slate-700">
        <div className="flex items-center gap-2">
          <Filter className="h-5 w-5 text-blue-600 dark:text-blue-400" />
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
            Expression Builder
          </h3>
          {!expressionIsValid() && expression.filters?.length === 0 && (
            <span className="text-xs text-amber-600 dark:text-amber-400">
              Incomplete
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          <div className="flex rounded-md border border-slate-200 dark:border-slate-700 overflow-hidden">
            <button
              onClick={() => setViewMode('visual')}
              className={`px-3 py-1 text-xs font-medium ${
                viewMode === 'visual'
                  ? 'bg-blue-600 text-white'
                  : 'bg-white dark:bg-slate-800 text-slate-600 dark:text-slate-400'
              }`}
            >
              Visual
            </button>
            <button
              onClick={() => setViewMode('json')}
              className={`px-3 py-1 text-xs font-medium border-l border-slate-200 dark:border-slate-700 ${
                viewMode === 'json'
                  ? 'bg-blue-600 text-white'
                  : 'bg-white dark:bg-slate-800 text-slate-600 dark:text-slate-400'
              }`}
            >
              JSON
            </button>
          </div>
          <button
            onClick={serializeExpression}
            className="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
            title="Copy expression JSON"
          >
            <Code className="h-4 w-4" />
          </button>
          <button
            onClick={() => setExpanded(!expanded)}
            className="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
          >
            {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          </button>
        </div>
      </div>

      {/* Content */}
      {expanded && (
        <div className="p-4 space-y-4">
          {viewMode === 'visual' ? (
            <>
              {/* Logic operator selector */}
              <div className="flex items-center gap-2">
                <span className="text-xs font-medium text-slate-500 dark:text-slate-400">
                  Match:
                </span>
                <select
                  value={expression.condition}
                  onChange={(e) => updateLogicOperator(e.target.value as LogicOperator)}
                  disabled={readOnly}
                  className="rounded-md border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-sm text-slate-900 dark:text-slate-100 px-2 py-1"
                >
                  {LOGIC_OPTIONS.map((opt) => (
                    <option key={opt.value} value={opt.value}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </div>

              {/* Filters */}
              {expression.filters && expression.filters.map((filter, index) => (
                <div key={index} className="flex items-center gap-2">
                  {!readOnly && (
                    <button
                      onClick={() => removeFilter(index)}
                      className="p-1 text-red-400 hover:text-red-600"
                      title="Remove filter"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  )}
                  <select
                    value={filter.field}
                    onChange={(e) => updateFilter(index, 'field', e.target.value)}
                    disabled={readOnly}
                    className="flex-1 rounded-md border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-sm text-slate-900 dark:text-slate-100 px-2 py-1"
                  >
                    <option value="">Select field...</option>
                    {FIELD_OPTIONS.map((opt) => (
                      <option key={opt.value} value={opt.value}>
                        {opt.label}
                      </option>
                    ))}
                  </select>
                  <select
                    value={filter.op}
                    onChange={(e) => updateFilter(index, 'op', e.target.value)}
                    disabled={readOnly}
                    className="flex-1 rounded-md border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-sm text-slate-900 dark:text-slate-100 px-2 py-1"
                  >
                    <option value="">Operator...</option>
                    {(Object.keys(OPERATOR_LABELS) as Operator[]).map((op) => (
                      <option key={op} value={op}>
                        {OPERATOR_LABELS[op]}
                      </option>
                    ))}
                  </select>
                  {filter.op !== 'is_null' && filter.op !== 'not_null' && (
                    <input
                      type="text"
                      value={typeof filter.value === 'string' ? filter.value : ''}
                      onChange={(e) => updateFilter(index, 'value', e.target.value)}
                      disabled={readOnly}
                      placeholder="Value..."
                      className="flex-1 rounded-md border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-700 text-sm text-slate-900 dark:text-slate-100 px-2 py-1"
                    />
                  )}
                  <Settings2 className="h-4 w-4 text-slate-400" />
                </div>
              ))}

              {/* Add buttons */}
              {!readOnly && (
                <div className="flex gap-2 pt-2 border-t border-slate-200 dark:border-slate-700">
                  <button
                    onClick={addFilter}
                    className="flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/20 rounded-md hover:bg-blue-100 dark:hover:bg-blue-900/30"
                  >
                    <Plus className="h-3 w-3" />
                    Add Filter
                  </button>
                  <button
                    onClick={addNestedGroup}
                    className="flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-purple-600 dark:text-purple-400 bg-purple-50 dark:bg-purple-900/20 rounded-md hover:bg-purple-100 dark:hover:bg-purple-900/30"
                  >
                    <Plus className="h-3 w-3" />
                    Nested Group
                  </button>
                </div>
              )}
            </>
          ) : (
            /* JSON View */
            <div className="relative">
              <pre className="text-xs font-mono bg-slate-50 dark:bg-slate-900 rounded-md p-3 text-slate-700 dark:text-slate-300 overflow-x-auto">
                {JSON.stringify(expression, null, 2)}
              </pre>
            </div>
          )}

          {/* Nested groups */}
          {expression.children && expression.children.length > 0 && viewMode === 'visual' && (
            <div className="border-t border-slate-200 dark:border-slate-700 pt-4 mt-4">
              <h4 className="text-xs font-medium text-slate-500 dark:text-slate-400 mb-2">
                Nested Groups
              </h4>
              {expression.children.map((child, index) => (
                <div key={index} className="ml-4 border-l-2 border-purple-200 dark:border-purple-800 pl-4 py-2">
                  <div className="flex items-center gap-2 text-xs">
                    <span className="font-medium text-purple-600 dark:text-purple-400">
                      {child.condition} Group {index + 1}
                    </span>
                    {!readOnly && (
                      <button
                        onClick={() => removeNested(index)}
                        className="text-red-400 hover:text-red-600"
                        title="Remove group"
                      >
                        <Trash2 className="h-3 w-3" />
                      </button>
                    )}
                  </div>
                  {child.filters && child.filters.length > 0 && (
                    <div className="mt-1 space-y-1">
                      {child.filters.map((f, fi) => (
                        <div key={fi} className="text-xs text-slate-500 dark:text-slate-400">
                          {f.field} {OPERATOR_LABELS[f.op as Operator]} {typeof f.value === 'string' ? `"${f.value}"` : String(f.value)}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
