import { useState, useEffect } from 'react';
import { useAuth } from '@/hooks/useAuth';
import { useToast } from '@/components/shared/Toast';
import { ConfirmDialog } from '@/components/shared/ConfirmDialog';
import { Skeleton } from '@/components/shared/Skeleton';

export default function ReportsPage() {
  const { user } = useAuth();
  const tenantID = user?.accessible_tenants?.[0]?.id || '';
  const [tab, setTab] = useState<'reports' | 'schedules'>('reports');
  const [reports, setReports] = useState<Record<string, unknown>[]>([]);
  const [schedules, setSchedules] = useState<Record<string, unknown>[]>([]);
  const [loading, setLoading] = useState(true);
  const [showSchedule, setShowSchedule] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  const load = async () => {
    if (!tenantID) return;
    setLoading(true);
    try {
      const [r, s] = await Promise.all([
        fetch(`/api/v1/reports/${tenantID}`).then(x => x.json()),
        fetch(`/api/v1/reports/${tenantID}/schedules`).then(x => x.json()),
      ]);
      setReports(r.reports || []);
      setSchedules(s.schedules || []);
    } catch (err) { console.error(err); }
    setLoading(false);
  };

  useEffect(() => { load(); }, [tenantID]);

  const handleGenerate = async () => {
    try {
      await fetch(`/api/v1/reports/${tenantID}/generate`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
      alert('Report generation started');
    } catch { alert('Failed'); }
  };

  const confirmScheduleDelete = (id: string) => {
    setConfirmDelete(id);
  };

  const doDeleteSchedule = async () => {
    if (!confirmDelete) return;
    const id = confirmDelete;
    setConfirmDelete(null);
    await fetch(`/api/v1/reports/${tenantID}/schedules/${id}`, { method: 'DELETE' });
    load();
  };

  if (loading) return <div className="text-center py-12 text-slate-500">Loading...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Reports</h1>
        <button onClick={handleGenerate} className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700">
          Generate Now
        </button>
      </div>

      <div className="border-b border-slate-200 dark:border-slate-700">
        <nav className="flex gap-1">
          <button onClick={() => setTab('reports')}
            className={`px-4 py-2 text-sm font-medium border-b-2 ${tab === 'reports' ? 'border-blue-600 text-blue-600' : 'border-transparent text-slate-500'}`}>
            Generated Reports ({reports.length})
          </button>
          <button onClick={() => setTab('schedules')}
            className={`px-4 py-2 text-sm font-medium border-b-2 ${tab === 'schedules' ? 'border-blue-600 text-blue-600' : 'border-transparent text-slate-500'}`}>
            Schedules ({schedules.length})
          </button>
        </nav>
      </div>

      {tab === 'reports' && (
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-800">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-slate-500">Name</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">Size</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">Format</th>
                <th className="text-right px-4 py-3 font-medium text-slate-500">Generated</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
              {reports.length === 0 ? (
                <tr><td colSpan={4} className="px-4 py-8 text-center text-slate-400">No reports generated</td></tr>
              ) : reports.map((r: Record<string, unknown>) => (
                <tr key={r.id as string}>
                  <td className="px-4 py-3 font-medium">{r.name as string}</td>
                  <td className="px-4 py-3 text-center">{(r.size_bytes as number) > 1024 ? `${((r.size_bytes as number) / 1024).toFixed(1)}KB` : `${r.size_bytes}B`}</td>
                  <td className="px-4 py-3 text-center uppercase text-xs font-mono">{r.format as string}</td>
                  <td className="px-4 py-3 text-right text-slate-500">{r.generated_at ? new Date(r.generated_at as string).toLocaleString() : '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'schedules' && (
        <div className="space-y-4">
          <button onClick={() => setShowSchedule(true)} className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700">
            + New Schedule
          </button>

          {schedules.length === 0 ? (
            <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-8 text-center text-slate-400">
              No schedules configured
            </div>
          ) : schedules.map((s: Record<string, unknown>) => (
            <div key={s.id as string} className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4 flex items-center justify-between">
              <div>
                <span className="font-medium">{s.name as string}</span>
                <span className="ml-2 px-2 py-0.5 rounded text-xs bg-slate-100 dark:bg-slate-700 capitalize">{s.frequency as string}</span>
                <span className={`ml-2 text-xs ${s.enabled ? 'text-green-600' : 'text-slate-400'}`}>{s.enabled ? 'Active' : 'Paused'}</span>
                {s.last_sent ? <span className="ml-2 text-xs text-slate-400">Last: {new Date(String(s.last_sent)).toLocaleDateString()}</span> : null}
              </div>
              <button onClick={() => confirmScheduleDelete(s.id as string)} className="text-red-600 hover:underline text-xs">Delete</button>
            </div>
          ))}
        </div>
      )}

      {showSchedule && (
        <CreateScheduleModal tenantID={tenantID} onClose={() => setShowSchedule(false)} onCreated={load} />
      )}

      <ConfirmDialog
        open={confirmDelete !== null}
        title="Delete Schedule"
        message="Are you sure you want to delete this report schedule?"
        onConfirm={doDeleteSchedule}
        onCancel={() => setConfirmDelete(null)}
      />
    </div>
  );
}

function CreateScheduleModal({ tenantID, onClose, onCreated }: {
  tenantID: string; onClose: () => void; onCreated: () => void;
}) {
  const [name, setName] = useState('');
  const [frequency, setFrequency] = useState('weekly');
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setCreating(true);
    try {
      const res = await fetch(`/api/v1/reports/${tenantID}/schedules`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, frequency, sections: ['summary', 'alerts', 'cves', 'patches'] }),
      });
      const data = await res.json();
      if (data.id) { onCreated(); onClose(); }
      else { setError(data.error || 'failed'); }
    } catch { setError('request failed'); }
    setCreating(false);
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-md w-full mx-4 p-6" onClick={e => e.stopPropagation()}>
        <form onSubmit={handleCreate} className="space-y-4">
          <h2 className="text-lg font-bold">New Report Schedule</h2>
          <div>
            <label className="block text-sm font-medium mb-1">Name</label>
            <input type="text" value={name} onChange={e => setName(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800" required />
          </div>
          <div>
            <label className="block text-sm font-medium mb-1">Frequency</label>
            <select value={frequency} onChange={e => setFrequency(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800">
              <option value="daily">Daily</option>
              <option value="weekly">Weekly</option>
              <option value="monthly">Monthly</option>
            </select>
          </div>
          {error && <p className="text-red-500 text-sm">{error}</p>}
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-4 py-2 text-sm rounded-md border hover:bg-slate-50">Cancel</button>
            <button type="submit" disabled={creating} className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 disabled:opacity-50">
              {creating ? 'Creating...' : 'Create Schedule'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
