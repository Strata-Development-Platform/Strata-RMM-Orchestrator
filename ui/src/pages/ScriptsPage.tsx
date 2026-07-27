import { useState, useEffect } from 'react';
import { useAuth } from '@/hooks/useAuth';
import { api } from '@/api/client';
import { useToast } from '@/components/shared/Toast';
import { ConfirmDialog } from '@/components/shared/ConfirmDialog';
import { EmptyState } from '@/components/shared/EmptyState';
import { Skeleton } from '@/components/shared/Skeleton';
import { StatusBadge } from '@/components/shared/StatusBadge';

type Script = Record<string, unknown>;
type Execution = Record<string, unknown>;

export default function ScriptsPage() {
  const { user } = useAuth();
  const { showToast } = useToast();
  const tenantID = user?.accessible_tenants?.[0]?.id || '';
  const [scripts, setScripts] = useState<Script[]>([]);
  const [executions, setExecutions] = useState<Execution[]>([]);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState<'library' | 'history'>('library');
  const [showCreate, setShowCreate] = useState(false);
  const [showRun, setShowRun] = useState<string | null>(null);
  const [showResult, setShowResult] = useState<string | null>(null);
  const [resultData, setResultData] = useState<Record<string, unknown> | null>(null);
  const [devices, setDevices] = useState<Record<string, unknown>[]>([]);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  const load = async () => {
    if (!tenantID) return;
    setLoading(true);
    try {
      const [s, e, d] = await Promise.all([
        api.getScripts(tenantID),
        api.getScriptExecutions(tenantID),
        api.getTenantDevices(tenantID).catch(() => ({ devices: [] })),
      ]);
      setScripts(s.scripts);
      setExecutions(e.executions);
      setDevices(d.devices);
    } catch (err) { console.error(err); }
    setLoading(false);
  };

  useEffect(() => { load(); }, [tenantID]);

  const handleDelete = async (scriptID: string) => {
    setConfirmDelete(scriptID);
  };

  const doDelete = async () => {
    if (!confirmDelete) return;
    await api.deleteScript(tenantID, confirmDelete);
    setConfirmDelete(null);
    showToast('success', 'Script deleted');
    load();
  };

  const handleViewResult = async (execID: string) => {
    try {
      const r = await api.getScriptExecution(tenantID, execID);
      setResultData(r);
      setShowResult(execID);
    } catch { showToast('error', 'Failed to load result'); }
  };

  if (loading) return <Skeleton type="table" rows={6} count={6} />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Scripting</h1>
        <button onClick={() => setShowCreate(true)}
          className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700">
          + New Script
        </button>
      </div>

      {/* Tabs */}
      <div className="border-b border-slate-200 dark:border-slate-700">
        <nav className="flex gap-1">
          <button onClick={() => setTab('library')}
            className={`px-4 py-2 text-sm font-medium border-b-2 ${tab === 'library' ? 'border-blue-600 text-blue-600' : 'border-transparent text-slate-500'}`}>
            Library ({scripts.length})
          </button>
          <button onClick={() => setTab('history')}
            className={`px-4 py-2 text-sm font-medium border-b-2 ${tab === 'history' ? 'border-blue-600 text-blue-600' : 'border-transparent text-slate-500'}`}>
            Execution History ({executions.length})
          </button>
        </nav>
      </div>

      {/* Script Library */}
      {tab === 'library' && (
        <div className="space-y-3">
          {scripts.length === 0 ? (
            <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-8 text-center text-slate-400">
              No scripts yet. Click "New Script" to create one.
            </div>
          ) : scripts.map((s: Script) => (
            <div key={s.id as string} className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <span className={`px-2 py-0.5 rounded text-xs font-mono font-medium ${
                    s.language === 'powershell' ? 'bg-blue-100 text-blue-800' :
                    s.language === 'bash' ? 'bg-green-100 text-green-800' :
                    s.language === 'python' ? 'bg-yellow-100 text-yellow-800' :
                    'bg-slate-100 text-slate-800'
                  }`}>{s.language as string}</span>
                  <span className="font-medium">{s.name as string}</span>
                  {s.description ? <span className="text-sm text-slate-500">— {String(s.description)}</span> : null}
                </div>
                <div className="flex items-center gap-2">
                  <button onClick={() => setShowRun(s.id as string)}
                    className="px-3 py-1 text-xs bg-green-600 text-white rounded hover:bg-green-700">Run</button>
                  <button onClick={() => handleDelete(s.id as string)}
                    className="px-3 py-1 text-xs bg-red-600 text-white rounded hover:bg-red-700">Delete</button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Execution History */}
      {tab === 'history' && (
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-800">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-slate-500">Status</th>
                <th className="text-left px-4 py-3 font-medium text-slate-500">Device</th>
                <th className="text-right px-4 py-3 font-medium text-slate-500">Duration</th>
                <th className="text-right px-4 py-3 font-medium text-slate-500">Exit Code</th>
                <th className="text-right px-4 py-3 font-medium text-slate-500">Time</th>
                <th className="text-right px-4 py-3 font-medium text-slate-500"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
              {executions.length === 0 ? (
                <tr><td colSpan={6} className="px-4 py-8 text-center text-slate-400">No executions</td></tr>
              ) : executions.map((e: Execution) => (
                <tr key={e.id as string}>
                  <td className="px-4 py-3">
                    <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                      e.status === 'success' ? 'bg-green-100 text-green-800' :
                      e.status === 'failed' ? 'bg-red-100 text-red-800' :
                      e.status === 'running' ? 'bg-blue-100 text-blue-800' :
                      e.status === 'timeout' ? 'bg-amber-100 text-amber-800' :
                      'bg-slate-100 text-slate-600'
                    }`}>{e.status as string}</span>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs">{(e.device_id as string)?.slice(0, 12)}...</td>
                  <td className="px-4 py-3 text-right">{e.duration_ms ? `${e.duration_ms}ms` : '-'}</td>
                  <td className="px-4 py-3 text-right font-mono">{e.exit_code != null ? String(e.exit_code) : '-'}</td>
                  <td className="px-4 py-3 text-right text-slate-500">
                    {e.created_at ? new Date(e.created_at as string).toLocaleString() : '-'}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button onClick={() => handleViewResult(e.id as string)}
                      className="text-blue-600 hover:underline text-xs">View</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create Script Modal */}
      {showCreate && <CreateScriptModal tenantID={tenantID} onClose={() => setShowCreate(false)} onCreated={load} />}

      {/* Run Script Modal */}
      {showRun && <RunScriptModal tenantID={tenantID} scriptID={showRun} devices={devices} onClose={() => setShowRun(null)} onRun={load} />}

      {/* Result Viewer */}
      {showResult && resultData && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => { setShowResult(null); setResultData(null); }}>
          <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-3xl w-full mx-4 max-h-[90vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
            <div className="p-6 space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-lg font-bold">Execution Result</h2>
                <button onClick={() => { setShowResult(null); setResultData(null); }} className="text-slate-400 hover:text-slate-600">✕</button>
              </div>

              <div className="flex gap-4 text-sm">
                <div><span className="text-slate-500">Status:</span> <span className={`font-medium ${
                  resultData.status === 'success' ? 'text-green-600' : 'text-red-600'
                }`}>{resultData.status as string}</span></div>
                <div><span className="text-slate-500">Exit Code:</span> <span className="font-mono">{resultData.exit_code as string}</span></div>
                <div><span className="text-slate-500">Duration:</span> <span>{resultData.duration_ms as string}ms</span></div>
              </div>

              <div>
                <h3 className="text-sm font-medium mb-1">stdout</h3>
                <pre className="bg-slate-900 text-green-400 p-4 rounded-lg text-xs overflow-x-auto max-h-60 overflow-y-auto font-mono whitespace-pre-wrap">
                  {(resultData.stdout as string) || '(no output)'}
                </pre>
              </div>
              {(resultData.stderr as string) && (
                <div>
                  <h3 className="text-sm font-medium mb-1">stderr</h3>
                  <pre className="bg-slate-900 text-red-400 p-4 rounded-lg text-xs overflow-x-auto max-h-40 overflow-y-auto font-mono whitespace-pre-wrap">
                    {resultData.stderr as string}
                  </pre>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function CreateScriptModal({ tenantID, onClose, onCreated }: {
  tenantID: string; onClose: () => void; onCreated: () => void;
}) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [language, setLanguage] = useState('powershell');
  const [content, setContent] = useState('');
  const [timeout, setTimeout_] = useState(300);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setCreating(true);
    try {
      await api.createScript(tenantID, { name, description, language, content, timeout_sec: timeout });
      onCreated();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'creation failed');
    }
    setCreating(false);
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-2xl w-full mx-4 max-h-[90vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <form onSubmit={handleCreate} className="p-6 space-y-4">
          <h2 className="text-lg font-bold">New Script</h2>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Name *</label>
              <input type="text" value={name} onChange={e => setName(e.target.value)}
                className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm" required />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Language</label>
              <select value={language} onChange={e => setLanguage(e.target.value)}
                className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm">
                <option value="powershell">PowerShell</option>
                <option value="bash">Bash</option>
                <option value="python">Python</option>
                <option value="batch">Batch</option>
              </select>
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Description</label>
            <input type="text" value={description} onChange={e => setDescription(e.target.value)}
              className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm" />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="text-sm font-medium">Script Content *</label>
              <span className="text-xs text-slate-400">Timeout: {timeout}s</span>
            </div>
            <textarea value={content} onChange={e => setContent(e.target.value)}
              className="w-full h-64 px-3 py-2 border rounded-md dark:bg-slate-800 text-sm font-mono" required
              placeholder={`Write your ${language} script here...\n\nUse {{param_name}} for parameters`} />
          </div>

          {error && <p className="text-red-500 text-sm">{error}</p>}

          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose}
              className="px-4 py-2 text-sm rounded-md border hover:bg-slate-50">Cancel</button>
            <button type="submit" disabled={creating}
              className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 disabled:opacity-50">
              {creating ? 'Creating...' : 'Create Script'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function RunScriptModal({ tenantID, scriptID, devices, onClose, onRun }: {
  tenantID: string; scriptID: string; devices: Record<string, unknown>[]; onClose: () => void; onRun: () => void;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const [params, setParams] = useState<Record<string, string>>({});
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');

  const toggleDevice = (id: string) => {
    setSelected(p => p.includes(id) ? p.filter(x => x !== id) : [...p, id]);
  };

  const handleRun = async (e: React.FormEvent) => {
    e.preventDefault();
    if (selected.length === 0) { setError('Select at least one device'); return; }
    setError('');
    setRunning(true);
    try {
      const res = await api.runScript(tenantID, scriptID, selected, params);
      alert(`Script dispatched to ${res.count} device(s)`);
      onRun();
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'run failed');
    }
    setRunning(false);
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <form onSubmit={handleRun} className="p-6 space-y-4">
          <h2 className="text-lg font-bold">Run Script</h2>

          <div>
            <label className="block text-sm font-medium mb-2">Target Devices ({selected.length} selected)</label>
            <div className="max-h-40 overflow-y-auto border rounded-md p-2 space-y-1">
              {devices.length === 0 && <p className="text-sm text-slate-400">No devices available</p>}
              {devices.map((d: Record<string, unknown>) => (
                <label key={d.id as string} className="flex items-center gap-2 text-sm cursor-pointer p-1 hover:bg-slate-50 dark:hover:bg-slate-800 rounded">
                  <input type="checkbox" checked={selected.includes(d.id as string)} onChange={() => toggleDevice(d.id as string)} />
                  <span>{d.hostname as string}</span>
                  <span className={`ml-auto text-xs ${d.status === 'online' ? 'text-green-600' : 'text-slate-400'}`}>{d.status as string}</span>
                </label>
              ))}
            </div>
          </div>

          {error && <p className="text-red-500 text-sm">{error}</p>}

          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose}
              className="px-4 py-2 text-sm rounded-md border hover:bg-slate-50">Cancel</button>
            <button type="submit" disabled={running || selected.length === 0}
              className="px-4 py-2 bg-green-600 text-white text-sm rounded-md hover:bg-green-700 disabled:opacity-50">
              {running ? 'Running...' : `Run on ${selected.length} device(s)`}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
