import { useCallback, useEffect, useRef, useState } from 'react';
import { RefreshCw, Save, ShieldCheck } from 'lucide-react';
import { api } from '@/api/client';
import type {
  ProviderBusinessProfile,
  ProviderBusinessProfilePatch,
  ProviderBusinessProfileValues,
} from '@/api/types';
import { useWorkspace } from '@/hooks/useWorkspace';
import {
  ALL_PROVIDER_FIELDS,
  EMPTY_PROVIDER_PROFILE,
  normalizeProviderProfile,
  validateProviderProfile,
  type ProviderProfileErrors,
  type ProviderProfileField,
} from '@/lib/providerProfile';
import {
  BusinessFields,
  ContactFields,
  ProfileErrorSummary,
  RegionalFields,
} from '@/components/provider/ProviderProfileFields';

function valuesFromProfile(profile: ProviderBusinessProfile): ProviderBusinessProfileValues {
  return {
    ...EMPTY_PROVIDER_PROFILE,
    legal_name: profile.legal_name,
    display_name: profile.display_name,
    contact_name: profile.contact_name,
    support_email: profile.support_email,
    billing_email: profile.billing_email,
    business_phone: profile.business_phone,
    website_url: profile.website_url ?? '',
    address_line1: profile.address_line1,
    address_line2: profile.address_line2 ?? '',
    city: profile.city,
    state_province: profile.state_province ?? '',
    postal_code: profile.postal_code,
    country_code: profile.country_code,
    default_timezone: profile.default_timezone,
    default_locale: profile.default_locale,
    default_currency: profile.default_currency,
    tax_identifier: profile.tax_identifier ?? '',
  };
}

function readableDate(value?: string) {
  if (!value) return 'Not recorded';
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString();
}

export function ProviderBusinessSettings() {
  const { workspace, applyProviderProfile } = useWorkspace();
  const canManage = Boolean(
    workspace && !workspace.msp_id &&
    workspace.platform_role,
  );
  const [profile, setProfile] = useState<ProviderBusinessProfile | null>(null);
  const [values, setValues] = useState<ProviderBusinessProfileValues>(EMPTY_PROVIDER_PROFILE);
  const [originalValues, setOriginalValues] = useState<ProviderBusinessProfileValues>(EMPTY_PROVIDER_PROFILE);
  const [errors, setErrors] = useState<ProviderProfileErrors>({});
  const [loadError, setLoadError] = useState('');
  const [saveError, setSaveError] = useState('');
  const [saveMessage, setSaveMessage] = useState('');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const errorSummaryRef = useRef<HTMLDivElement>(null);

  const loadProfile = useCallback(async () => {
    if (!canManage) return;
    setLoading(true);
    setLoadError('');
    try {
      const result = await api.getProviderProfile();
      const nextValues = valuesFromProfile(result);
      setProfile(result);
      setValues(nextValues);
      setOriginalValues(nextValues);
    } catch (caught) {
      setLoadError(caught instanceof Error ? caught.message : 'Provider business profile could not be loaded.');
    } finally {
      setLoading(false);
    }
  }, [canManage]);

  useEffect(() => {
    void loadProfile();
  }, [loadProfile]);

  if (!canManage) return null;

  const normalizedValues = normalizeProviderProfile(values);
  const normalizedOriginal = normalizeProviderProfile(originalValues);
  const dirty = ALL_PROVIDER_FIELDS.some(field => normalizedValues[field] !== normalizedOriginal[field]);

  const handleChange = (field: ProviderProfileField, value: string) => {
    setValues(current => ({ ...current, [field]: value }));
    setErrors(current => {
      if (!current[field]) return current;
      const next = { ...current };
      delete next[field];
      return next;
    });
    setSaveError('');
    setSaveMessage('');
  };

  const handleSave = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!dirty || saving) return;
    const nextErrors = validateProviderProfile(values);
    setErrors(nextErrors);
    setSaveError('');
    setSaveMessage('');
    if (Object.keys(nextErrors).length > 0) {
      window.requestAnimationFrame(() => errorSummaryRef.current?.focus());
      return;
    }

    const patch: ProviderBusinessProfilePatch = {};
    for (const field of ALL_PROVIDER_FIELDS) {
      if (normalizedValues[field] !== normalizedOriginal[field]) patch[field] = normalizedValues[field];
    }

    setSaving(true);
    try {
      const result = await api.updateProviderProfile(patch);
      const savedValues = valuesFromProfile(result);
      setProfile(result);
      setValues(savedValues);
      setOriginalValues(savedValues);
      setSaveMessage('Business profile saved. The update was recorded in the provider audit trail.');
      applyProviderProfile(result);
    } catch (caught) {
      setSaveError(caught instanceof Error ? caught.message : 'Provider business profile could not be saved.');
      window.requestAnimationFrame(() => errorSummaryRef.current?.focus());
    } finally {
      setSaving(false);
    }
  };

  return (
    <section aria-labelledby="provider-business-heading" className="rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-col gap-4 border-b border-slate-200 px-5 py-5 dark:border-slate-800 sm:flex-row sm:items-start sm:justify-between sm:px-6">
        <div>
          <div className="flex items-center gap-2">
            <ShieldCheck size={20} className="text-blue-600 dark:text-blue-400" />
            <h2 id="provider-business-heading" className="text-lg font-semibold text-slate-950 dark:text-white">Provider business profile</h2>
          </div>
          <p className="mt-1 max-w-2xl text-sm leading-6 text-slate-500 dark:text-slate-400">Manage the business identity, contact details, and regional defaults established during provider setup.</p>
        </div>
        <div className="text-xs text-slate-500 dark:text-slate-400 sm:text-right">
          <p>Last updated</p>
          <p className="mt-0.5 font-medium text-slate-700 dark:text-slate-300">{readableDate(profile?.updated_at)}</p>
        </div>
      </div>

      {loading && (
        <div className="flex items-center gap-2 px-5 py-10 text-sm text-slate-500 sm:px-6">
          <RefreshCw size={16} className="animate-spin" /> Loading provider profile...
        </div>
      )}

      {!loading && loadError && (
        <div className="p-5 sm:p-6">
          <div role="alert" className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-900 dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">
            <p className="font-semibold">Provider profile unavailable</p>
            <p className="mt-1">{loadError}</p>
            <button type="button" onClick={() => void loadProfile()} className="mt-3 inline-flex items-center gap-1.5 rounded-md bg-red-700 px-3 py-2 font-medium text-white hover:bg-red-800">
              <RefreshCw size={15} /> Try again
            </button>
          </div>
        </div>
      )}

      {!loading && !loadError && profile && (
        <form noValidate onSubmit={handleSave}>
          <div className="space-y-8 p-5 sm:p-6">
            <div ref={errorSummaryRef} tabIndex={-1} className="outline-none">
              <ProfileErrorSummary title="Business profile could not be saved" errors={errors} idPrefix="settings-provider" message={saveError} />
            </div>
            {saveMessage && <p role="status" className="rounded-lg border border-green-200 bg-green-50 p-4 text-sm text-green-900 dark:border-green-900 dark:bg-green-950/40 dark:text-green-200">{saveMessage}</p>}

            <fieldset disabled={saving} className="space-y-8">
              <legend className="sr-only">Editable provider business profile</legend>
              <div>
                <h3 className="mb-4 text-base font-semibold text-slate-900 dark:text-white">Business</h3>
                <BusinessFields values={values} errors={errors} onChange={handleChange} disabled={saving} idPrefix="settings-provider" />
              </div>
              <div className="border-t border-slate-200 pt-8 dark:border-slate-800">
                <h3 className="mb-4 text-base font-semibold text-slate-900 dark:text-white">Contact</h3>
                <ContactFields values={values} errors={errors} onChange={handleChange} disabled={saving} idPrefix="settings-provider" />
              </div>
              <div className="border-t border-slate-200 pt-8 dark:border-slate-800">
                <h3 className="mb-4 text-base font-semibold text-slate-900 dark:text-white">Regional defaults</h3>
                <RegionalFields values={values} errors={errors} onChange={handleChange} disabled={saving} idPrefix="settings-provider" />
              </div>
            </fieldset>

            <div className="rounded-lg bg-slate-50 p-4 text-sm text-slate-600 dark:bg-slate-950/60 dark:text-slate-400">
              <p><span className="font-medium text-slate-800 dark:text-slate-200">Setup completed:</span> {readableDate(profile.setup_completed_at)}</p>
              <p className="mt-1">Saving records changed field names in the provider audit trail. Setup completion time and actor are immutable.</p>
            </div>
          </div>

          <div className="flex flex-col-reverse gap-3 border-t border-slate-200 bg-slate-50 px-5 py-4 dark:border-slate-800 dark:bg-slate-900/70 sm:flex-row sm:items-center sm:justify-between sm:px-6">
            <button type="button" onClick={() => { setValues(originalValues); setErrors({}); setSaveError(''); setSaveMessage(''); }} disabled={!dirty || saving} className="rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200 dark:hover:bg-slate-800">
              Discard changes
            </button>
            <button type="submit" disabled={!dirty || saving} className="inline-flex items-center justify-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50">
              {saving ? <><RefreshCw size={16} className="animate-spin" /> Saving...</> : <><Save size={16} /> Save business profile</>}
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
