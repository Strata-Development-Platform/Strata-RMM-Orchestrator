import { useState, useEffect } from 'react';
import { api } from '@/api/client';

type Job = Record<string, unknown>;
type Target = Record<string, unknown>;

const STATUS_STYLES: Record<string, string> = {
  pending: 'bg-slate-100 text-slate-800', queued: 'bg-blue-100 text-blue-800',
  dispatched: 'bg-indigo-100 text-indigo-800', running: 'bg-amber-100 text-amber-800',
  succeeded: 'bg-green-100 text-green-800', failed: 'bg-red-100 text-red-800',
  cancelled: 'bg-zinc-100 text-zinc-600', expired: 'bg-orange-100 text-orange-800',
};

export default function JobsPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [filterType, setFilterType] = useState('');
  const [filterStatus, setFilterStatus] = useState('');
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);
  const [targets, setTargets] = useState<Target[]>([]);
  const [jobDetail, setJobDetail] = useState<Job | null>(null);

  const loadJobs = async () => {
    setLoading(true);
    setError('');
    try {
      let url = '/api/v1/jobs';
      if (filterStatus) url += `&status=${filterStatus}`;
      if (filterType) url += `&type=${filterType}`;
      const res = await fetch(url, { headers: { 'Authorization': `Bearer ${api.getToken()}` } });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Failed to load jobs');
      if (data.jobs) setJobs(data.jobs);
      else setError(data.error || 'Failed to load');
    } catch (err) { setError(err instanceof Error ? err.message : 'Connection failed'); }
    setLoading(false);
  };

  useEffect(() => { loadJobs(); }, [filterStatus, filterType]);

  const openJob = async (job: Job) => {
    setSelectedJob(job);
    setJobDetail(job);
    try {
      const res = await fetch(`/api/v1/jobs/${job.id}`, {
        headers: { 'Authorization': `Bearer ${api.getToken()}` },
      });
      const data = await res.json();
      setJobDetail(data);
      setTargets(data.targets || []);
    } catch { setTargets([]); }
  };

  const cancelJob = async (jobId: string) => {
    const res = await fetch(`/api/v1/jobs/${jobId}/cancel`, {
      method: 'POST', headers: { 'Authorization': `Bearer ${api.getToken()}` },
    });
    const data = await res.json();
    if (!res.ok) {
      setError(data.error || 'Cancellation failed');
      return;
    }
    loadJobs();
    setSelectedJob(null);
  };

  const retryJob = async (jobId: string) => {
    const res = await fetch(`/api/v1/jobs/${jobId}/retry`, {
      method: 'POST', headers: { 'Authorization': `Bearer ${api.getToken()}` },
    });
    const data = await res.json();
    if (!res.ok) {
      setError(data.error || 'Retry failed');
      return;
    }
    loadJobs();
  };

  const terminalStates = ['succeeded', 'failed', 'cancelled', 'expired'];
  const activeJobs = jobs.filter(j => !terminalStates.includes(j.status as string));
  const completedJobs = jobs.filter(j => terminalStates.includes(j.status as string));

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Job Center</h1>
        <button onClick={loadJobs} className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700">
          Refresh
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3">
        <select value={filterStatus} onChange={e => setFilterStatus(e.target.value)}
          className="px-3 py-2 border rounded-md text-sm dark:bg-slate-800">
          <option value="">All statuses</option>
          {['queued','dispatched','running','succeeded','failed','cancelled','expired'].map(s =>
            <option key={s} value={s}>{s}</option>
          )}
        </select>
        <select value={filterType} onChange={e => setFilterType(e.target.value)}
          className="px-3 py-2 border rounded-md text-sm dark:bg-slate-800">
          <option value="">All types</option>
          {[...new Set(jobs.map(j => j.type as string))].filter(Boolean).map(t =>
            <option key={t} value={t}>{t}</option>
          )}
        </select>
      </div>

      {error && <div className="bg-red-50 dark:bg-red-900/20 text-red-600 p-4 rounded-lg text-sm">{error}</div>}

      {/* Active Jobs */}
      {!selectedJob && (
        <div className="space-y-4">
          {activeJobs.length > 0 && (
            <div>
              <h2 className="text-lg font-semibold mb-3">Active ({activeJobs.length})</h2>
              <div className="space-y-2">{activeJobs.map(job => jobCard(job, openJob))}</div>
            </div>
          )}
          <div>
            <h2 className="text-lg font-semibold mb-3">{activeJobs.length > 0 ? 'Recent' : 'All Jobs'} ({completedJobs.length})</h2>
            {loading ? (
              <div className="text-center py-12 text-slate-500">Loading...</div>
            ) : jobs.length === 0 ? (
              <div className="text-center py-12 text-slate-400">No jobs found</div>
            ) : (
              <div className="space-y-2">{completedJobs.slice(0, 20).map(job => jobCard(job, openJob))}</div>
            )}
          </div>
        </div>
      )}

      {/* Job Detail */}
      {selectedJob && jobDetail && (
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-6">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-3">
              <button onClick={() => setSelectedJob(null)} className="text-slate-400 hover:text-slate-600">&larr; Back</button>
              <h2 className="text-lg font-bold">{jobDetail.type as string}</h2>
              <span className={`px-2 py-0.5 rounded text-xs font-medium ${STATUS_STYLES[jobDetail.status as string] || 'bg-slate-100 text-slate-800'}`}>
                {jobDetail.status as string}
              </span>
            </div>
            <div className="flex gap-2">
              {!terminalStates.includes(jobDetail.status as string) && (
                <button onClick={() => cancelJob(jobDetail.id as string)}
                  className="px-3 py-1 text-xs bg-red-600 text-white rounded hover:bg-red-700">Cancel</button>
              )}
              {(jobDetail.status as string) === 'failed' && (
                <button onClick={() => retryJob(jobDetail.id as string)}
                  className="px-3 py-1 text-xs bg-green-600 text-white rounded hover:bg-green-700">Retry</button>
              )}
            </div>
          </div>

          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-4 text-sm">
            <div><span className="text-slate-500">Job ID:</span> <span className="font-mono">{(jobDetail.id as string)?.slice(0, 12)}...</span></div>
            <div><span className="text-slate-500">Created:</span> <span>{jobDetail.created_at ? new Date(jobDetail.created_at as string).toLocaleString() : '-'}</span></div>
            <div><span className="text-slate-500">Type:</span> <span>{jobDetail.type as string}</span></div>
            <div><span className="text-slate-500">Status:</span> <span>{jobDetail.status as string}</span></div>
          </div>

          {/* Targets */}
          <h3 className="font-semibold mb-3">Targets ({targets.length})</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-slate-50 dark:bg-slate-800">
                <tr>
                  <th className="text-left px-3 py-2 font-medium text-slate-500">Device</th>
                  <th className="text-center px-3 py-2 font-medium text-slate-500">Status</th>
                  <th className="text-center px-3 py-2 font-medium text-slate-500">Attempt</th>
                  <th className="text-center px-3 py-2 font-medium text-slate-500">Exit</th>
                  <th className="text-right px-3 py-2 font-medium text-slate-500">Error</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                {targets.length === 0 ? (
                  <tr><td colSpan={5} className="px-3 py-8 text-center text-slate-400">No targets</td></tr>
                ) : targets.map((t, i) => (
                  <tr key={i}>
                    <td className="px-3 py-2 font-mono text-xs">{(t.device_id as string)?.slice(0, 16)}...</td>
                    <td className="px-3 py-2 text-center">
                      <span className={`px-2 py-0.5 rounded text-xs font-medium ${STATUS_STYLES[t.status as string] || ''}`}>{t.status as string}</span>
                    </td>
                    <td className="px-3 py-2 text-center">{t.attempt as number || 0}</td>
                    <td className="px-3 py-2 text-center font-mono ">{t.exit_code != null ? String(t.exit_code) : '-'}</td>
                    <td className="px-3 py-2 text-right text-red-500 text-xs max-w-xs truncate">{t.error_message as string || ''}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

function jobCard(job: Job, onClick: (j: Job) => void) {
  return (
    <div key={job.id as string}
      className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4 flex items-center justify-between cursor-pointer hover:bg-slate-50 dark:hover:bg-slate-800/50 transition-colors"
      onClick={() => onClick(job)}>
      <div className="flex items-center gap-3">
        <span className={`w-2 h-2 rounded-full ${statusDot(job.status as string)}`} />
        <div>
          <span className="font-medium text-sm">{job.type as string}</span>
          <span className="ml-2 text-xs text-slate-400">{(job.id as string)?.slice(0, 12)}...</span>
        </div>
      </div>
      <div className="flex items-center gap-3">
        <span className={`px-2 py-0.5 rounded text-xs font-medium ${STATUS_STYLES[job.status as string] || 'bg-slate-100'}`}>
          {job.status as string}
        </span>
        <span className="text-xs text-slate-400">{job.created_at ? new Date(job.created_at as string).toLocaleString() : ''}</span>
        <span className="text-blue-600 text-sm">&rarr;</span>
      </div>
    </div>
  );
}

function statusDot(status: string): string {
  switch (status) {
    case 'queued': case 'dispatched': return 'bg-blue-400';
    case 'running': return 'bg-amber-400 animate-pulse';
    case 'succeeded': return 'bg-green-400';
    case 'failed': return 'bg-red-400';
    case 'cancelled': return 'bg-zinc-400';
    case 'expired': return 'bg-orange-400';
    default: return 'bg-slate-300';
  }
}
