import { useEffect, useState, type FormEvent } from 'react';
import { api } from '@/api/client';
import type {
  BrandingProfile,
  ClientOrganization,
  CustomDomain,
  EnrollmentToken,
  Entitlement,
  Membership,
  ManagedDevice,
  Site,
  Usage,
} from '@/api/types';
import { useWorkspace } from '@/hooks/useWorkspace';
import { useToast } from '@/components/shared/Toast';

type Tab = 'overview' | 'clients' | 'devices' | 'members' | 'branding' | 'domains' | 'enrollment' | 'audit';

export default function MSPWorkspacePage() {
  const { workspace, loading: contextLoading, refresh: refreshContext } = useWorkspace();
  const { showToast } = useToast();
  const [tab, setTab] = useState<Tab>('overview');
  const [clients, setClients] = useState<ClientOrganization[]>([]);
  const [sites, setSites] = useState<Record<string, Site[]>>({});
  const [memberships, setMemberships] = useState<Membership[]>([]);
  const [branding, setBranding] = useState<BrandingProfile | null>(null);
  const [domains, setDomains] = useState<CustomDomain[]>([]);
  const [tokens, setTokens] = useState<EnrollmentToken[]>([]);
  const [entitlement, setEntitlement] = useState<Entitlement | null>(null);
  const [usage, setUsage] = useState<Usage | null>(null);
  const [audit, setAudit] = useState<Record<string, unknown>[]>([]);
  const [devices, setDevices] = useState<ManagedDevice[]>([]);
  const [busy, setBusy] = useState(false);

  const mspID = workspace?.msp_id || '';
  const canManage = workspace?.permissions.some(permission =>
    ['platform:manage', 'msp:manage'].includes(permission)
  ) ?? false;

  const reload = async () => {
    if (!mspID) return;
    setBusy(true);
    try {
      const [clientData, memberData, brandData, domainData, tokenData, entitlementData, usageData, auditData, deviceData] =
        await Promise.all([
          api.getClients(mspID),
          api.getMemberships(mspID),
          api.getBranding(),
          api.getDomains(),
          api.getEnrollmentTokens(),
          api.getEntitlement(mspID),
          api.getUsage(mspID),
          api.getControlPlaneAudit(mspID),
          api.getMSPDevices(mspID),
        ]);
      setClients(clientData.clients);
      setMemberships(memberData.memberships);
      setBranding(brandData);
      setDomains(domainData.domains);
      setTokens(tokenData.tokens);
      setEntitlement(entitlementData);
      setUsage(usageData);
      setAudit(auditData.entries);
      setDevices(deviceData.devices);
    } catch (error) {
      showToast('error', error instanceof Error ? error.message : 'Unable to load workspace');
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    void reload();
    // reload is intentionally keyed to the selected tenant.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mspID]);

  const perform = async (action: () => Promise<unknown>, message: string) => {
    setBusy(true);
    try {
      await action();
      showToast('success', message);
      await reload();
    } catch (error) {
      showToast('error', error instanceof Error ? error.message : 'Action failed');
      setBusy(false);
    }
  };

  if (contextLoading) return <p className="py-12 text-center text-slate-500">Loading workspace…</p>;
  if (!mspID) {
    return (
      <div className="rounded-xl border border-slate-200 bg-white p-8 text-center dark:border-slate-700 dark:bg-slate-900">
        <h1 className="text-xl font-semibold">Select an MSP workspace</h1>
        <p className="mt-2 text-slate-500">Use the workspace selector in the sidebar to begin.</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm font-medium text-blue-600 dark:text-blue-400">SaaS Control Plane</p>
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">{workspace?.msp_name}</h1>
        <p className="text-sm text-slate-500">{entitlement?.plan_slug || 'No plan'} · {entitlement?.status || 'unknown'}</p>
      </div>

      <nav className="flex flex-wrap gap-2" aria-label="MSP workspace sections">
        {(['overview', 'clients', 'devices', 'members', 'branding', 'domains', 'enrollment', 'audit'] as Tab[]).map(item => (
          <button
            key={item}
            onClick={() => setTab(item)}
            className={`rounded-md px-3 py-2 text-sm font-medium capitalize ${
              tab === item ? 'bg-blue-600 text-white' : 'bg-white text-slate-600 dark:bg-slate-900 dark:text-slate-300'
            }`}
          >
            {item}
          </button>
        ))}
      </nav>

      {busy && <div className="h-1 overflow-hidden rounded bg-blue-100"><div className="h-full w-2/3 animate-pulse bg-blue-600" /></div>}

      {tab === 'overview' && (
        <Overview usage={usage} entitlement={entitlement} canManage={canManage} mspID={mspID} perform={perform} />
      )}
      {tab === 'clients' && (
        <ClientsPanel clients={clients} sites={sites} setSites={setSites} mspID={mspID} canManage={canManage} perform={perform} />
      )}
      {tab === 'devices' && <DevicesPanel devices={devices} />}
      {tab === 'members' && (
        <MembersPanel memberships={memberships} mspID={mspID} canManage={canManage} perform={perform} />
      )}
      {tab === 'branding' && branding && (
        <BrandingPanel
          branding={branding}
          canManage={canManage}
          onSave={value => perform(async () => {
            await api.updateBranding(value);
            await refreshContext();
          }, 'Branding updated')}
        />
      )}
      {tab === 'domains' && (
        <DomainsPanel domains={domains} canManage={canManage} perform={perform} />
      )}
      {tab === 'enrollment' && (
        <EnrollmentPanel clients={clients} sites={sites} setSites={setSites} tokens={tokens} canManage={canManage} perform={perform} />
      )}
      {tab === 'audit' && <AuditPanel entries={audit} />}
    </div>
  );
}

function DevicesPanel({ devices }: { devices: ManagedDevice[] }) {
  return <div className="overflow-hidden rounded-xl border bg-white dark:border-slate-700 dark:bg-slate-900">
    <table className="w-full text-sm"><thead className="bg-slate-50 dark:bg-slate-800"><tr><th className="px-4 py-3 text-left">Device</th><th className="px-4 py-3 text-left">Client / Site</th><th className="px-4 py-3 text-left">Agent</th><th className="px-4 py-3 text-left">Status</th><th className="px-4 py-3 text-right">Operations</th></tr></thead><tbody className="divide-y dark:divide-slate-700">{devices.map(device => <tr key={device.id}><td className="px-4 py-3"><p className="font-medium">{device.hostname}</p><p className="text-xs text-slate-500">{device.os} {device.arch}</p></td><td className="px-4 py-3">{device.client_name}<span className="text-slate-400"> / </span>{device.site_name}</td><td className="px-4 py-3">{device.agent_version || 'Unknown'}</td><td className="px-4 py-3 capitalize">{device.status}</td><td className="px-4 py-3 text-right"><a className={secondaryButton} href={`/remote/${device.tenant_id}/${device.id}`}>Remote</a><a className={`${secondaryButton} ml-2`} href="/jobs">Jobs</a><a className={`${secondaryButton} ml-2`} href="/scripts">Scripts</a></td></tr>)}</tbody></table>
    {devices.length === 0 && <Empty label="No enrolled devices." />}
  </div>;
}

function Overview({ usage, entitlement, canManage, mspID, perform }: {
  usage: Usage | null;
  entitlement: Entitlement | null;
  canManage: boolean;
  mspID: string;
  perform: (action: () => Promise<unknown>, message: string) => Promise<void>;
}) {
  const cards = [
    ['Devices', usage?.device_count ?? 0, entitlement?.max_devices],
    ['Users', usage?.user_count ?? 0, entitlement?.max_users],
    ['Clients', usage?.client_count ?? 0, undefined],
    ['Sites', usage?.site_count ?? 0, undefined],
  ] as const;
  return (
    <div className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {cards.map(([label, value, maximum]) => (
          <div key={label} className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-900">
            <p className="text-sm text-slate-500">{label}</p>
            <p className="mt-1 text-3xl font-bold">{value}{maximum !== undefined && <span className="text-base font-normal text-slate-400"> / {maximum}</span>}</p>
          </div>
        ))}
      </div>
      {canManage && entitlement && (
        <form
          className="flex flex-wrap items-end gap-3 rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-900"
          onSubmit={event => {
            event.preventDefault();
            const data = new FormData(event.currentTarget);
            void perform(
              () => api.updateEntitlement(mspID, String(data.get('plan')), String(data.get('status'))),
              'Subscription updated',
            );
          }}
        >
          <Field label="Plan"><select name="plan" defaultValue={entitlement.plan_slug} className={inputClass}><option value="free">Free</option><option value="starter">Starter</option><option value="professional">Professional</option></select></Field>
          <Field label="Status"><select name="status" defaultValue={entitlement.status} className={inputClass}><option value="active">Active</option><option value="past_due">Past due</option><option value="suspended">Suspended</option><option value="cancelled">Cancelled</option></select></Field>
          <button className={primaryButton}>Update subscription</button>
        </form>
      )}
    </div>
  );
}

function ClientsPanel({ clients, sites, setSites, mspID, canManage, perform }: {
  clients: ClientOrganization[];
  sites: Record<string, Site[]>;
  setSites: (value: Record<string, Site[]>) => void;
  mspID: string;
  canManage: boolean;
  perform: (action: () => Promise<unknown>, message: string) => Promise<void>;
}) {
  const loadSites = async (clientID: string) => {
    const result = await api.getSites(clientID);
    setSites({ ...sites, [clientID]: result.sites });
  };
  return (
    <div className="space-y-4">
      {canManage && <InlineCreate labels={['Name', 'Slug']} onSubmit={(name, slug) => perform(() => api.createClient(mspID, name, slug), 'Client created')} />}
      {clients.map(client => (
        <div key={client.id} className="rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-900">
          <div className="flex items-center justify-between gap-3">
            <div><h3 className="font-semibold">{client.name}</h3><p className="text-sm text-slate-500">{client.slug}</p></div>
            <div className="flex gap-2">
              <button className={secondaryButton} onClick={() => void loadSites(client.id)}>Sites</button>
              {canManage && <button className={dangerButton} onClick={() => void perform(() => api.archiveClient(mspID, client.id), 'Client archived')}>Archive</button>}
            </div>
          </div>
          {sites[client.id] && (
            <div className="mt-4 space-y-2 border-t border-slate-200 pt-4 dark:border-slate-700">
              {canManage && <InlineCreate labels={['Site name', 'Site slug']} onSubmit={(name, slug) => perform(() => api.createSite(client.id, name, slug), 'Site created')} compact />}
              {sites[client.id].map(site => (
                <div key={site.id} className="flex items-center justify-between rounded bg-slate-50 px-3 py-2 text-sm dark:bg-slate-800">
                  <span>{site.name} <span className="text-slate-500">· {site.device_count || 0} devices</span></span>
                  {canManage && <button className="text-red-600" onClick={() => void perform(() => api.archiveSite(client.id, site.id), 'Site archived')}>Archive</button>}
                </div>
              ))}
              {sites[client.id].length === 0 && <p className="text-sm text-slate-500">No sites yet.</p>}
            </div>
          )}
        </div>
      ))}
      {clients.length === 0 && <Empty label="No clients have been created." />}
    </div>
  );
}

function MembersPanel({ memberships, mspID, canManage, perform }: {
  memberships: Membership[]; mspID: string; canManage: boolean;
  perform: (action: () => Promise<unknown>, message: string) => Promise<void>;
}) {
  return <div className="space-y-4">
    {canManage && (
      <form className="flex flex-wrap items-end gap-3 rounded-xl border bg-white p-4 dark:border-slate-700 dark:bg-slate-900" onSubmit={(event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        void perform(() => api.createMembership(mspID, String(data.get('user')), String(data.get('role'))), 'Member added');
        event.currentTarget.reset();
      }}>
        <Field label="User ID"><input required name="user" className={inputClass} /></Field>
        <Field label="Role"><select name="role" className={inputClass}><option value="msp_admin">MSP admin</option><option value="msp_technician">Technician</option><option value="msp_viewer">Viewer</option></select></Field>
        <button className={primaryButton}>Add member</button>
      </form>
    )}
    {memberships.map(member => <div key={member.id} className="flex items-center justify-between rounded-xl border bg-white p-4 dark:border-slate-700 dark:bg-slate-900"><div><p className="font-medium">{member.user_id}</p><p className="text-sm text-slate-500">{member.role}</p></div>{canManage && <button className={dangerButton} onClick={() => void perform(() => api.revokeMembership(mspID, member.id), 'Membership revoked')}>Revoke</button>}</div>)}
    {memberships.length === 0 && <Empty label="No active MSP memberships." />}
  </div>;
}

function BrandingPanel({ branding, canManage, onSave }: { branding: BrandingProfile; canManage: boolean; onSave: (value: Partial<BrandingProfile>) => Promise<void> }) {
  const [value, setValue] = useState(branding);
  return <form className="grid gap-4 rounded-xl border bg-white p-5 sm:grid-cols-2 dark:border-slate-700 dark:bg-slate-900" onSubmit={event => { event.preventDefault(); void onSave(value); }}>
    {([
      ['display_name', 'Display name'], ['portal_title', 'Portal title'], ['welcome_text', 'Welcome text'],
      ['logo_light', 'Light logo URL'], ['logo_dark', 'Dark logo URL'], ['favicon', 'Favicon URL'],
      ['support_email', 'Support email'], ['terms_url', 'Terms URL'], ['privacy_url', 'Privacy URL'],
    ] as const).map(([key, label]) => <Field key={key} label={label}><input disabled={!canManage} value={value[key] || ''} onChange={event => setValue({ ...value, [key]: event.target.value })} className={inputClass} /></Field>)}
    {([
      ['primary_color', 'Primary'], ['accent_color', 'Accent'], ['sidebar_bg', 'Sidebar'],
      ['header_bg', 'Header'], ['login_bg', 'Login background'],
    ] as const).map(([key, label]) => <Field key={key} label={label}><input disabled={!canManage} type="color" value={value[key]} onChange={event => setValue({ ...value, [key]: event.target.value })} className="h-10 w-full rounded border p-1 dark:border-slate-600 dark:bg-slate-800" /></Field>)}
    {canManage && <div className="sm:col-span-2"><button className={primaryButton}>Save and apply branding</button></div>}
  </form>;
}

function DomainsPanel({ domains, canManage, perform }: { domains: CustomDomain[]; canManage: boolean; perform: (action: () => Promise<unknown>, message: string) => Promise<void> }) {
  const { showToast } = useToast();
  return <div className="space-y-4">
    {canManage && <form className="flex gap-3 rounded-xl border bg-white p-4 dark:border-slate-700 dark:bg-slate-900" onSubmit={event => {
      event.preventDefault();
      const form = event.currentTarget;
      const hostname = String(new FormData(form).get('hostname'));
      void (async () => {
        try {
          const result = await api.createDomain(hostname);
          await navigator.clipboard.writeText(result.verification_token);
          showToast('success', `Domain added. TXT token copied for ${result.txt_name}`);
          form.reset();
          await perform(() => Promise.resolve(), 'Domain ready for verification');
        } catch (error) {
          showToast('error', error instanceof Error ? error.message : 'Unable to add domain');
        }
      })();
    }}><input required name="hostname" placeholder="rmm.example.com" className={`${inputClass} flex-1`} /><button className={primaryButton}>Add domain</button></form>}
    {domains.map(domain => <div key={domain.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-white p-4 dark:border-slate-700 dark:bg-slate-900"><div><p className="font-medium">{domain.hostname}</p><p className="text-sm text-slate-500">{domain.verification_status} · certificate {domain.certificate_status}</p>{domain.verification_token && <button className="mt-1 text-left font-mono text-xs text-blue-600" onClick={() => void navigator.clipboard.writeText(domain.verification_token || '')}>TXT {domain.txt_name}: {domain.verification_token} (copy)</button>}</div>{canManage && <div className="flex gap-2">{['pending', 'failed'].includes(domain.verification_status) && <button className={secondaryButton} onClick={() => void perform(() => api.verifyDomain(domain.id), 'Domain verified; certificate requested')}>Verify DNS</button>}<button className={dangerButton} onClick={() => void perform(() => api.deleteDomain(domain.id), 'Domain removed')}>Remove</button></div>}</div>)}
    {domains.length === 0 && <Empty label="No custom domains configured." />}
  </div>;
}

function EnrollmentPanel({ clients, sites, setSites, tokens, canManage, perform }: {
  clients: ClientOrganization[]; sites: Record<string, Site[]>; setSites: (value: Record<string, Site[]>) => void;
  tokens: EnrollmentToken[]; canManage: boolean; perform: (action: () => Promise<unknown>, message: string) => Promise<void>;
}) {
  const { showToast } = useToast();
  const [clientID, setClientID] = useState('');
  const create = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    try {
      const result = await api.createEnrollmentToken(clientID, String(data.get('site')), Number(data.get('uses')), String(data.get('description')));
      await navigator.clipboard.writeText(result.token);
      showToast('success', 'One-time enrollment token copied to clipboard');
      await perform(() => Promise.resolve(), 'Enrollment token created');
    } catch (error) {
      showToast('error', error instanceof Error ? error.message : 'Unable to create token');
    }
  };
  return <div className="space-y-4">
    {canManage && <form className="grid gap-3 rounded-xl border bg-white p-4 md:grid-cols-4 dark:border-slate-700 dark:bg-slate-900" onSubmit={event => void create(event)}>
      <Field label="Client"><select required value={clientID} onChange={async event => { const id = event.target.value; setClientID(id); if (id && !sites[id]) { const result = await api.getSites(id); setSites({ ...sites, [id]: result.sites }); } }} className={inputClass}><option value="">Select client</option>{clients.map(client => <option key={client.id} value={client.id}>{client.name}</option>)}</select></Field>
      <Field label="Site"><select name="site" className={inputClass}><option value="">Any active site</option>{(sites[clientID] || []).map(site => <option key={site.id} value={site.id}>{site.name}</option>)}</select></Field>
      <Field label="Maximum uses"><input name="uses" type="number" min="1" max="100" defaultValue="1" className={inputClass} /></Field>
      <Field label="Description"><input name="description" placeholder="Ticket or installer" className={inputClass} /></Field>
      <div className="md:col-span-4"><button className={primaryButton}>Generate deployment token</button></div>
    </form>}
    {tokens.map(token => <div key={token.id} className="flex items-center justify-between rounded-xl border bg-white p-4 dark:border-slate-700 dark:bg-slate-900"><div><p className="font-medium">{token.client_id}</p><p className="text-sm text-slate-500">{token.use_count}/{token.max_uses} uses · expires {new Date(token.expires_at).toLocaleString()}</p></div>{canManage && !token.is_revoked && <button className={dangerButton} onClick={() => void perform(() => api.revokeEnrollmentToken(token.id), 'Token revoked')}>Revoke</button>}</div>)}
    {tokens.length === 0 && <Empty label="No enrollment tokens." />}
  </div>;
}

function AuditPanel({ entries }: { entries: Record<string, unknown>[] }) {
  return <div className="overflow-hidden rounded-xl border bg-white dark:border-slate-700 dark:bg-slate-900"><table className="w-full text-sm"><thead className="bg-slate-50 dark:bg-slate-800"><tr><th className="px-4 py-3 text-left">Time</th><th className="px-4 py-3 text-left">Action</th><th className="px-4 py-3 text-left">Resource</th><th className="px-4 py-3 text-left">Actor</th></tr></thead><tbody className="divide-y dark:divide-slate-700">{entries.map(entry => <tr key={String(entry.id)}><td className="px-4 py-3">{new Date(String(entry.created_at)).toLocaleString()}</td><td className="px-4 py-3">{String(entry.action)}</td><td className="px-4 py-3">{String(entry.resource_type)} · {String(entry.resource_id)}</td><td className="px-4 py-3">{String(entry.actor_user_id)}</td></tr>)}</tbody></table>{entries.length === 0 && <Empty label="No control-plane audit entries." />}</div>;
}

function InlineCreate({ labels, onSubmit, compact = false }: { labels: [string, string]; onSubmit: (name: string, slug: string) => Promise<void>; compact?: boolean }) {
  return <form className={`flex flex-wrap gap-2 ${compact ? 'mb-3' : 'rounded-xl border bg-white p-4 dark:border-slate-700 dark:bg-slate-900'}`} onSubmit={event => { event.preventDefault(); const form = event.currentTarget; const data = new FormData(form); void onSubmit(String(data.get('name')), String(data.get('slug'))).then(() => form.reset()); }}><input required name="name" placeholder={labels[0]} className={inputClass} /><input required name="slug" placeholder={labels[1]} pattern="[a-z0-9-]+" className={inputClass} /><button className={primaryButton}>Create</button></form>;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="block text-sm"><span className="mb-1 block font-medium text-slate-600 dark:text-slate-300">{label}</span>{children}</label>;
}

function Empty({ label }: { label: string }) {
  return <p className="p-8 text-center text-sm text-slate-500">{label}</p>;
}

const inputClass = 'rounded-md border border-slate-300 bg-white px-3 py-2 text-sm dark:border-slate-600 dark:bg-slate-800';
const primaryButton = 'rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50';
const secondaryButton = 'rounded-md bg-slate-100 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-200';
const dangerButton = 'rounded-md bg-red-50 px-3 py-2 text-sm font-medium text-red-700 hover:bg-red-100 dark:bg-red-950/40 dark:text-red-300';
