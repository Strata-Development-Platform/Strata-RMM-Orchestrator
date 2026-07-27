import { useState, useEffect } from 'react';
import { api } from '@/api/client';

export default function JobHealthPage() {
  const [stats, setStats] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetch('/api/v1/jobs', {
      headers: { 'Authorization': `Bearer ${api.getToken()}` },
    }).then(async r => {
      const data = await r.json();
      if (!r.ok) throw new Error(data.error || 'Failed to load job health');
      return data;
    }).then(data => {
      if (data.jobs) {
        const s: Record<string, number> = {};
        data.jobs.forEach((j: Record<string, unknown>) => {
          const st = j.status as string || 'unknown';
          s[st] = (s[st] || 0) + 1;
        });
        setStats(s);
      }
      setLoading(false);
    }).catch(() => setLoading(false));
  }, []);

  if (loading) return <div className="text-center py-12 text-slate-500">Loading...</div>;

  const cards = [
    { label: 'Failed', value: stats['failed'] || 0, color: 'text-red-600' },
    { label: 'Expired', value: stats['expired'] || 0, color: 'text-orange-600' },
    { label: 'Running', value: stats['running'] || 0, color: 'text-amber-600' },
    { label: 'Queued', value: (stats['queued']||0)+(stats['dispatched']||0), color: 'text-blue-600' },
    { label: 'Succeeded', value: stats['succeeded'] || 0, color: 'text-green-600' },
    { label: 'Cancelled', value: stats['cancelled'] || 0, color: 'text-zinc-600' },
  ];

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Job Health</h1>
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        {cards.map(c => (
          <div key={c.label} className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
            <p className="text-sm text-slate-500">{c.label}</p>
            <p className={`text-2xl font-bold mt-1 ${c.color}`}>{c.value}</p>
          </div>
        ))}
      </div>
      {stats['failed'] > 0 && (
        <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
          <h2 className="font-semibold text-red-800 dark:text-red-300">{stats['failed']} failed job(s) — check Job Center</h2>
        </div>
      )}
    </div>
  );
}
