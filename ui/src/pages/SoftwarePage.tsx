import { useState, useEffect } from 'react';
import { useAuth } from '@/hooks/useAuth';
import { api } from '@/api/client';
import { useToast } from '@/components/shared/Toast';
import { ConfirmDialog } from '@/components/shared/ConfirmDialog';
import { Skeleton } from '@/components/shared/Skeleton';

export default function SoftwarePage() {
  const { user } = useAuth();
  const tenantID = user?.accessible_tenants?.[0]?.id || '';
  const [packages, setPackages] = useState<Record<string, unknown>[]>([]);
  const [deployments, setDeployments] = useState<Record<string, unknown>[]>([]);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState<'packages' | 'deployments'>('packages');
  const [showCreatePkg, setShowCreatePkg] = useState(false);
  const [showDeploy, setShowDeploy] = useState<string | null>(null);
  const [showDetail, setShowDetail] = useState<string | null>(null);
  const [detailData, setDetailData] = useState<Record<string, unknown> | null>(null);
  const [devices, setDevices] = useState<Record<string, unknown>[]>([]);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  const load = async () => {
    if (!tenantID) return;
    setLoading(true);
    try {
      const [p, d, dev] = await Promise.all([
        api.getPackages(tenantID),
        api.getDeployments(tenantID),
        api.getTenantDevices(tenantID).catch(() => ({ devices: [] })),
      ]);
      setPackages(p.packages);
      setDeployments(d.deployments);
      setDevices(dev.devices);
    } catch (err) { console.error(err); }
    setLoading(false);
  };

  useEffect(() => { load(); }, [tenantID]);

  const confirmPkgDelete = (id: string) => setConfirmDelete(id);

  const doDeletePkg = async () => {
    if (!confirmDelete) return;
    await api.deletePackage(tenantID, confirmDelete);
    setConfirmDelete(null);
    showToast('success', 'Package deleted');
    load();
  };

  const handleViewDetail = async (id: string) => {
    try {
      const d = await api.getDeployment(tenantID, id);
      setDetailData(d);
      setShowDetail(id);
    } catch { showToast('error', 'Failed to load'); }
  };

  if (loading) return <Skeleton type="table" rows={5} count={6} />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">Software Deployment</h1>
        {tab === 'packages' && (
          <button onClick={() => setShowCreatePkg(true)}
            className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700">
            + Add Package
          </button>
        )}
      </div>

      <div className="border-b border-slate-200 dark:border-slate-700">
        <nav className="flex gap-1">
          <button onClick={() => setTab('packages')}
            className={`px-4 py-2 text-sm font-medium border-b-2 ${tab === 'packages' ? 'border-blue-600 text-blue-600' : 'border-transparent text-slate-500'}`}>
            Packages ({packages.length})
          </button>
          <button onClick={() => setTab('deployments')}
            className={`px-4 py-2 text-sm font-medium border-b-2 ${tab === 'deployments' ? 'border-blue-600 text-blue-600' : 'border-transparent text-slate-500'}`}>
            Deployments ({deployments.length})
          </button>
        </nav>
      </div>

      {/* Package Library */}
      {tab === 'packages' && (
        <div className="space-y-3">
          {packages.length === 0 ? (
            <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-8 text-center text-slate-400">
              No packages. Add an MSI, EXE, DEB, or RPM to deploy.
            </div>
          ) : packages.map((p: Record<string, unknown>) => (
            <div key={p.id as string} className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <span className={`px-2 py-0.5 rounded text-xs font-mono font-medium ${
                    p.package_type === 'msi' ? 'bg-blue-100 text-blue-800' :
                    p.package_type === 'exe' ? 'bg-purple-100 text-purple-800' :
                    p.package_type === 'deb' ? 'bg-orange-100 text-orange-800' :
                    p.package_type === 'rpm' ? 'bg-red-100 text-red-800' :
                    'bg-slate-100 text-slate-800'
                  }`}>{(p.package_type as string)?.toUpperCase()}</span>
                  <span className="font-medium">{p.name as string}</span>
                  {p.version && <span className="text-sm text-slate-500">v{p.version as string}</span>}
                  {p.description && <span className="text-sm text-slate-400 truncate max-w-xs">— {p.description as string}</span>}
                </div>
                <div className="flex items-center gap-2">
                  <button onClick={() => setShowDeploy(p.id as string)}
                    className="px-3 py-1 text-xs bg-green-600 text-white rounded hover:bg-green-700">Deploy</button>
                  <button onClick={() => confirmPkgDelete(p.id as string)}
                    className="px-3 py-1 text-xs bg-red-600 text-white rounded hover:bg-red-700">Delete</button>
                </div>
              </div>
              <div className="mt-2 text-xs text-slate-400 font-mono truncate">{p.source_url as string}</div>
            </div>
          ))}
        </div>
      )}

      {/* Deployments */}
      {tab === 'deployments' && (
        <div className="bg-white dark:bg-slate-900 rounded-lg border border-slate-200 dark:border-slate-700 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 dark:bg-slate-800">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-slate-500">Name</th>
                <th className="text-left px-4 py-3 font-medium text-slate-500">Package</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">Status</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">Success</th>
                <th className="text-center px-4 py-3 font-medium text-slate-500">Failed</th>
                <th className="text-right px-4 py-3 font-medium text-slate-500">Created</th>
                <th className="text-right px-4 py-3 font-medium text-slate-500"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
              {deployments.length === 0 ? (
                <tr><td colSpan={7} className="px-4 py-8 text-center text-slate-400">No deployments</td></tr>
              ) : deployments.map((d: Record<string, unknown>) => (
                <tr key={d.id as string}>
                  <td className="px-4 py-3 font-medium">{d.name as string}</td>
                  <td className="px-4 py-3">{d.package_name as string}</td>
                  <td className="px-4 py-3 text-center">
                    <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                      d.status === 'completed' ? 'bg-green-100 text-green-800' :
                      d.status === 'deploying' ? 'bg-blue-100 text-blue-800' :
                      d.status === 'failed' ? 'bg-red-100 text-red-800' :
                      'bg-slate-100 text-slate-600'
                    }`}>{d.status as string}</span>
                  </td>
                  <td className="px-4 py-3 text-center text-green-600">{d.success_count as number ?? 0}</td>
                  <td className="px-4 py-3 text-center text-red-600">{d.fail_count as number ?? 0}</td>
                  <td className="px-4 py-3 text-right text-slate-500">
                    {d.created_at ? new Date(d.created_at as string).toLocaleDateString() : '-'}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button onClick={() => handleViewDetail(d.id as string)}
                      className="text-blue-600 hover:underline text-xs">Detail</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create Package Modal */}
      {showCreatePkg && <CreatePackageModal tenantID={tenantID} onClose={() => setShowCreatePkg(false)} onCreated={load} />}

      {/* Deploy Modal */}
      {showDeploy && (
        <DeployModal
          tenantID={tenantID}
          packageID={showDeploy}
          devices={devices}
          onClose={() => setShowDeploy(null)}
          onDeploy={load}
        />
      )}

      <ConfirmDialog
        open={confirmDelete !== null}
        title="Delete Package"
        message="Are you sure you want to delete this package? Deployments using it will be orphaned."
        onConfirm={doDeletePkg}
        onCancel={() => setConfirmDelete(null)}
      />

      {/* Deployment Detail */}
      {showDetail && detailData && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => { setShowDetail(null); setDetailData(null); }}>
          <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-2xl w-full mx-4 max-h-[90vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
            <div className="p-6 space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-lg font-bold">Deployment: {(detailData.name as string) || detailData.id as string}</h2>
                <button onClick={() => { setShowDetail(null); setDetailData(null); }} className="text-slate-400 hover:text-slate-600">✕</button>
              </div>

              <div className="text-sm"><span className="text-slate-500">Status:</span> <span className="font-medium">{detailData.status as string}</span></div>

              <table className="w-full text-sm">
                <thead className="bg-slate-50 dark:bg-slate-800">
                  <tr>
                    <th className="text-left px-4 py-2 font-medium text-slate-500">Device</th>
                    <th className="text-center px-4 py-2 font-medium text-slate-500">Status</th>
                    <th className="text-right px-4 py-2 font-medium text-slate-500">Duration</th>
                    <th className="text-right px-4 py-2 font-medium text-slate-500">Error</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-200 dark:divide-slate-700">
                  {((detailData.targets as Record<string, unknown>[]) || []).map((t: Record<string, unknown>) => (
                    <tr key={t.device_id as string}>
                      <td className="px-4 py-2 font-mono text-xs">{t.hostname as string || (t.device_id as string)?.slice(0, 12)}</td>
                      <td className="px-4 py-2 text-center">
                        <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                          t.status === 'success' ? 'bg-green-100 text-green-800' :
                          t.status === 'failed' ? 'bg-red-100 text-red-800' :
                          t.status === 'downloading' || t.status === 'installing' ? 'bg-blue-100 text-blue-800' :
                          'bg-slate-100 text-slate-600'
                        }`}>{t.status as string}</span>
                      </td>
                      <td className="px-4 py-2 text-right">{(t.duration_ms as number) ? `${t.duration_ms}ms` : '-'}</td>
                      <td className="px-4 py-2 text-right text-red-500 text-xs max-w-xs truncate">{(t.error_message as string) || ''}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function CreatePackageModal({ tenantID, onClose, onCreated }: {
  tenantID: string; onClose: () => void; onCreated: () => void;
}) {
  const [name, setName] = useState('');
  const [version, setVersion] = useState('');
  const [description, setDescription] = useState('');
  const [platform, setPlatform] = useState('all');
  const [packageType, setPackageType] = useState('msi');
  const [sourceURL, setSourceURL] = useState('');
  const [checksum, setChecksum] = useState('');
  const [installArgs, setInstallArgs] = useState('');
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setCreating(true);
    try {
      await api.createPackage(tenantID, {
        name, version, description, platform,
        package_type: packageType, source_url: sourceURL,
        checksum, install_args: installArgs,
      });
      onCreated();
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'creation failed');
    }
    setCreating(false);
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-xl w-full mx-4 max-h-[90vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <form onSubmit={handleCreate} className="p-6 space-y-4">
          <h2 className="text-lg font-bold">Add Package</h2>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Name *</label>
              <input type="text" value={name} onChange={e => setName(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm" required />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Version</label>
              <input type="text" value={version} onChange={e => setVersion(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm" placeholder="1.0.0" />
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Source URL *</label>
            <input type="url" value={sourceURL} onChange={e => setSourceURL(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm font-mono" placeholder="https://example.com/app.msi" required />
          </div>

          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Type</label>
              <select value={packageType} onChange={e => setPackageType(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm">
                <option value="msi">MSI</option>
                <option value="exe">EXE</option>
                <option value="deb">DEB</option>
                <option value="rpm">RPM</option>
                <option value="appimage">AppImage</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">Platform</label>
              <select value={platform} onChange={e => setPlatform(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm">
                <option value="all">All</option>
                <option value="windows">Windows</option>
                <option value="linux">Linux</option>
                <option value="macos">macOS</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">SHA256</label>
              <input type="text" value={checksum} onChange={e => setChecksum(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-xs font-mono" placeholder="Optional" />
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Description</label>
            <input type="text" value={description} onChange={e => setDescription(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm" />
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">Install Arguments</label>
            <input type="text" value={installArgs} onChange={e => setInstallArgs(e.target.value)} className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm font-mono" placeholder="/quiet /norestart" />
          </div>

          {error && <p className="text-red-500 text-sm">{error}</p>}

          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-4 py-2 text-sm rounded-md border hover:bg-slate-50">Cancel</button>
            <button type="submit" disabled={creating}
              className="px-4 py-2 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 disabled:opacity-50">
              {creating ? 'Adding...' : 'Add Package'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function DeployModal({ tenantID, packageID, devices, onClose, onDeploy }: {
  tenantID: string; packageID: string; devices: Record<string, unknown>[]; onClose: () => void; onDeploy: () => void;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const [action, setAction] = useState('install');
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');

  const toggle = (id: string) => setSelected(p => p.includes(id) ? p.filter(x => x !== id) : [...p, id]);

  const handleDeploy = async (e: React.FormEvent) => {
    e.preventDefault();
    if (selected.length === 0) { setError('Select devices'); return; }
    setError('');
    setRunning(true);
    try {
      const res = await api.createDeployment(tenantID, {
        package_id: packageID, device_ids: selected, action,
      });
      alert(`Deploying to ${(res.targets as number) || selected.length} device(s)`);
      onDeploy();
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'failed');
    }
    setRunning(false);
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-slate-900 rounded-lg shadow-xl max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <form onSubmit={handleDeploy} className="p-6 space-y-4">
          <h2 className="text-lg font-bold">Deploy Software</h2>

          <div>
            <label className="block text-sm font-medium mb-1">Action</label>
            <select value={action} onChange={e => setAction(e.target.value)}
              className="w-full px-3 py-2 border rounded-md dark:bg-slate-800 text-sm">
              <option value="install">Install</option>
              <option value="uninstall">Uninstall</option>
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium mb-2">Target Devices ({selected.length})</label>
            <div className="max-h-40 overflow-y-auto border rounded-md p-2 space-y-1">
              {devices.map((d: Record<string, unknown>) => (
                <label key={d.id as string} className="flex items-center gap-2 text-sm cursor-pointer p-1 rounded hover:bg-slate-50 dark:hover:bg-slate-800">
                  <input type="checkbox" checked={selected.includes(d.id as string)} onChange={() => toggle(d.id as string)} />
                  <span>{d.hostname as string}</span>
                  <span className={`ml-auto text-xs ${d.status === 'online' ? 'text-green-600' : 'text-slate-400'}`}>{d.status as string}</span>
                </label>
              ))}
            </div>
          </div>

          {error && <p className="text-red-500 text-sm">{error}</p>}

          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-4 py-2 text-sm rounded-md border hover:bg-slate-50">Cancel</button>
            <button type="submit" disabled={running || selected.length === 0}
              className="px-4 py-2 bg-green-600 text-white text-sm rounded-md hover:bg-green-700 disabled:opacity-50">
              {running ? 'Deploying...' : `Deploy to ${selected.length} device(s)`}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
