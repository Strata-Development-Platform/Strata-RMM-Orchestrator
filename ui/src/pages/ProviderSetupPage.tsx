import { useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { Building2, Check, ChevronLeft, ChevronRight, ClipboardCheck, Globe2, LogOut, UserRound } from 'lucide-react';
import { api } from '@/api/client';
import { ProductAttribution } from '@/components/layout/ProductAttribution';
import { ThemeToggle } from '@/components/shared/ThemeToggle';
import {
  BusinessFields,
  ContactFields,
  ProfileErrorSummary,
  RegionalFields,
} from '@/components/provider/ProviderProfileFields';
import { useAuth } from '@/hooks/useAuth';
import { useWorkspace } from '@/hooks/useWorkspace';
import {
  BUSINESS_FIELDS,
  CONTACT_FIELDS,
  EMPTY_PROVIDER_PROFILE,
  PROVIDER_FIELD_LABELS,
  REGIONAL_FIELDS,
  TIMEZONE_OPTIONS,
  firstErrorStep,
  normalizeProviderProfile,
  validateProviderProfile,
  type ProviderProfileErrors,
  type ProviderProfileField,
} from '@/lib/providerProfile';
import type { ProviderBusinessProfileValues } from '@/api/types';

const steps = [
  { name: 'Business', description: 'Business identity', icon: Building2 },
  { name: 'Contact', description: 'Support and billing', icon: UserRound },
  { name: 'Regional Defaults', description: 'Address and locale', icon: Globe2 },
  { name: 'Review', description: 'Confirm and finish', icon: ClipboardCheck },
];

function initialValues(): ProviderBusinessProfileValues {
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return {
    ...EMPTY_PROVIDER_PROFILE,
    default_timezone: TIMEZONE_OPTIONS.includes(timezone) ? timezone : 'UTC',
  };
}

function ReviewGroup({ title, fields, values, onEdit }: {
  title: string;
  fields: ProviderProfileField[];
  values: ProviderBusinessProfileValues;
  onEdit: () => void;
}) {
  return (
    <section className="rounded-lg border border-slate-200 bg-slate-50/70 p-4 dark:border-slate-800 dark:bg-slate-950/50">
      <div className="flex items-center justify-between gap-3">
        <h3 className="font-semibold text-slate-900 dark:text-white">{title}</h3>
        <button type="button" onClick={onEdit} className="text-sm font-medium text-blue-700 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300">
          Edit {title.toLowerCase()}
        </button>
      </div>
      <dl className="mt-3 grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
        {fields.map(field => (
          <div key={field} className={field === 'address_line1' || field === 'address_line2' || field === 'website_url' ? 'sm:col-span-2' : ''}>
            <dt className="text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">{PROVIDER_FIELD_LABELS[field]}</dt>
            <dd className="mt-0.5 break-words text-sm text-slate-900 dark:text-slate-200">{values[field] || 'Not provided'}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

export default function ProviderSetupPage() {
  const navigate = useNavigate();
  const { logout } = useAuth();
  const { workspace, applyProviderProfile } = useWorkspace();
  const [step, setStep] = useState(0);
  const [values, setValues] = useState<ProviderBusinessProfileValues>(initialValues);
  const [errors, setErrors] = useState<ProviderProfileErrors>({});
  const [submissionError, setSubmissionError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const errorSummaryRef = useRef<HTMLDivElement>(null);

  const focusErrorSummary = () => {
    window.requestAnimationFrame(() => errorSummaryRef.current?.focus());
  };

  const handleChange = (field: ProviderProfileField, value: string) => {
    setValues(current => ({ ...current, [field]: value }));
    setErrors(current => {
      if (!current[field]) return current;
      const next = { ...current };
      delete next[field];
      return next;
    });
    setSubmissionError('');
  };

  const stepFields = step === 0 ? BUSINESS_FIELDS : step === 1 ? CONTACT_FIELDS : REGIONAL_FIELDS;

  const continueToNextStep = () => {
    const nextErrors = validateProviderProfile(values, stepFields);
    setErrors(nextErrors);
    setSubmissionError('');
    if (Object.keys(nextErrors).length > 0) {
      focusErrorSummary();
      return;
    }
    setStep(current => Math.min(current + 1, steps.length - 1));
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (submitting) return;
    const nextErrors = validateProviderProfile(values);
    setErrors(nextErrors);
    setSubmissionError('');
    if (Object.keys(nextErrors).length > 0) {
      setStep(firstErrorStep(nextErrors));
      focusErrorSummary();
      return;
    }

    setSubmitting(true);
    try {
      const profile = await api.completeProviderSetup(normalizeProviderProfile(values));
      applyProviderProfile(profile);
      navigate('/', { replace: true });
    } catch (caught) {
      setSubmissionError(caught instanceof Error ? caught.message : 'Provider setup could not be completed. Please retry.');
      focusErrorSummary();
    } finally {
      setSubmitting(false);
    }
  };

  const handleLogout = () => {
    logout();
    navigate('/login', { replace: true });
  };

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900 dark:bg-slate-950 dark:text-slate-100">
      <header className="border-b border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3 sm:px-6">
          <div className="min-w-0">
            <p className="truncate font-semibold">{workspace?.provider_display_name || 'Strata RMM'}</p>
            <p className="text-xs text-slate-500 dark:text-slate-400">Provider setup</p>
          </div>
          <div className="flex items-center gap-1">
            <ThemeToggle />
            <button type="button" onClick={handleLogout} className="inline-flex items-center gap-1.5 rounded-md px-3 py-2 text-sm text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-300 dark:hover:bg-slate-800 dark:hover:text-white">
              <LogOut size={16} /> <span className="hidden sm:inline">Sign out</span>
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-8 sm:px-6 sm:py-12">
        <div className="mx-auto max-w-3xl text-center">
          <span className="inline-flex rounded-full bg-blue-100 px-3 py-1 text-xs font-semibold uppercase tracking-wide text-blue-800 dark:bg-blue-950 dark:text-blue-300">First-time setup</span>
          <h1 className="mt-4 text-3xl font-bold tracking-tight text-slate-950 dark:text-white sm:text-4xl">Set up your provider business profile</h1>
          <p className="mx-auto mt-3 max-w-2xl text-sm leading-6 text-slate-600 dark:text-slate-400 sm:text-base">These details identify your provider workspace and establish regional defaults. You can update editable profile fields later in Platform Settings.</p>
        </div>

        <ol aria-label="Setup progress" className="mx-auto mt-8 grid max-w-4xl grid-cols-4 gap-1 sm:gap-3">
          {steps.map((item, index) => {
            const Icon = item.icon;
            const complete = index < step;
            const current = index === step;
            return (
              <li key={item.name} aria-current={current ? 'step' : undefined} className="min-w-0">
                <div className={`h-1 rounded-full ${index <= step ? 'bg-blue-600' : 'bg-slate-200 dark:bg-slate-800'}`} />
                <div className="mt-3 flex items-start gap-2">
                  <span className={`hidden rounded-md p-1.5 sm:inline-flex ${current ? 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300' : complete ? 'bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300' : 'bg-slate-100 text-slate-400 dark:bg-slate-900'}`}>
                    {complete ? <Check size={16} /> : <Icon size={16} />}
                  </span>
                  <span className="min-w-0">
                    <span className={`block truncate text-xs font-semibold sm:text-sm ${current ? 'text-blue-700 dark:text-blue-300' : 'text-slate-600 dark:text-slate-400'}`}>{item.name}</span>
                    <span className="hidden text-xs text-slate-500 md:block">{item.description}</span>
                  </span>
                </div>
              </li>
            );
          })}
        </ol>

        <form noValidate onSubmit={handleSubmit} className="mx-auto mt-8 max-w-4xl rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
          <div className="border-b border-slate-200 px-5 py-5 dark:border-slate-800 sm:px-8">
            <p className="text-sm font-medium text-blue-700 dark:text-blue-400">Step {step + 1} of {steps.length}</p>
            <h2 className="mt-1 text-xl font-semibold text-slate-950 dark:text-white">{steps[step].name}</h2>
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{steps[step].description}</p>
          </div>

          <div className="space-y-6 p-5 sm:p-8">
            <div ref={errorSummaryRef} tabIndex={-1} className="outline-none">
              <ProfileErrorSummary
                title={submissionError ? 'Setup could not be completed' : 'Please correct the highlighted fields'}
                errors={errors}
                message={submissionError}
              />
            </div>

            {step === 0 && <BusinessFields values={values} errors={errors} onChange={handleChange} disabled={submitting} />}
            {step === 1 && <ContactFields values={values} errors={errors} onChange={handleChange} disabled={submitting} />}
            {step === 2 && <RegionalFields values={values} errors={errors} onChange={handleChange} disabled={submitting} />}
            {step === 3 && (
              <div className="space-y-4">
                <ReviewGroup title="Business" fields={BUSINESS_FIELDS} values={values} onEdit={() => setStep(0)} />
                <ReviewGroup title="Contact" fields={CONTACT_FIELDS} values={values} onEdit={() => setStep(1)} />
                <ReviewGroup title="Regional defaults" fields={REGIONAL_FIELDS} values={values} onEdit={() => setStep(2)} />
                <p className="rounded-lg bg-blue-50 p-4 text-sm leading-6 text-blue-900 dark:bg-blue-950/40 dark:text-blue-200">Completing setup records who completed the provider profile and when. Those completion details remain unchanged by later profile edits.</p>
              </div>
            )}
          </div>

          <div className="flex items-center justify-between gap-3 border-t border-slate-200 bg-slate-50 px-5 py-4 dark:border-slate-800 dark:bg-slate-900/70 sm:px-8">
            <button type="button" onClick={() => setStep(current => Math.max(0, current - 1))} disabled={step === 0 || submitting} className="inline-flex items-center gap-1.5 rounded-md border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200 dark:hover:bg-slate-800">
              <ChevronLeft size={16} /> Back
            </button>
            {step < steps.length - 1 ? (
              <button key="continue" type="button" onClick={continueToNextStep} className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700">
                Continue <ChevronRight size={16} />
              </button>
            ) : (
              <button key="submit" type="submit" disabled={submitting} className="inline-flex min-w-36 items-center justify-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-60">
                {submitting ? 'Completing setup...' : 'Complete setup'}
              </button>
            )}
          </div>
        </form>
      </main>

      <footer className="mx-auto max-w-6xl px-4 pb-6 sm:px-6">
        <ProductAttribution collapsed={false} />
      </footer>
    </div>
  );
}
