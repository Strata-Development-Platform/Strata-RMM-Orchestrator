import type { ReactNode } from 'react';
import type { ProviderBusinessProfileValues } from '@/api/types';
import {
  COUNTRY_CODES,
  CURRENCY_CODES,
  LOCALE_OPTIONS,
  PROVIDER_FIELD_LABELS,
  TIMEZONE_OPTIONS,
  countryOptionLabel,
  type ProviderProfileErrors,
  type ProviderProfileField,
} from '@/lib/providerProfile';

type FieldsProps = {
  values: ProviderBusinessProfileValues;
  errors: ProviderProfileErrors;
  onChange: (field: ProviderProfileField, value: string) => void;
  disabled?: boolean;
  idPrefix?: string;
};

const inputClass = 'w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 shadow-sm outline-none transition focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-500 dark:border-slate-700 dark:bg-slate-950 dark:text-white dark:disabled:bg-slate-800';

function FormField({
  field, errors, idPrefix = 'provider', required = false, helper, children,
}: {
  field: ProviderProfileField;
  errors: ProviderProfileErrors;
  idPrefix?: string;
  required?: boolean;
  helper?: string;
  children: ReactNode;
}) {
  const id = `${idPrefix}-${field}`;
  const errorID = `${id}-error`;
  const helperID = `${id}-helper`;
  return (
    <div>
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">
        {PROVIDER_FIELD_LABELS[field]}{required && <span className="text-red-600" aria-hidden="true"> *</span>}
        {required && <span className="sr-only"> (required)</span>}
      </label>
      {children}
      {helper && !errors[field] && <p id={helperID} className="mt-1 text-xs text-slate-500 dark:text-slate-400">{helper}</p>}
      {errors[field] && <p id={errorID} className="mt-1 text-sm text-red-600 dark:text-red-400">{errors[field]}</p>}
    </div>
  );
}

function inputA11y(field: ProviderProfileField, errors: ProviderProfileErrors, idPrefix: string, helper = false) {
  const id = `${idPrefix}-${field}`;
  return {
    id,
    'aria-invalid': errors[field] ? true : undefined,
    'aria-describedby': errors[field] ? `${id}-error` : helper ? `${id}-helper` : undefined,
  };
}

export function BusinessFields({ values, errors, onChange, disabled = false, idPrefix = 'provider' }: FieldsProps) {
  return (
    <div className="grid grid-cols-1 gap-5 md:grid-cols-2">
      <FormField field="legal_name" errors={errors} idPrefix={idPrefix} required>
        <input {...inputA11y('legal_name', errors, idPrefix)} className={inputClass} value={values.legal_name} onChange={event => onChange('legal_name', event.target.value)} disabled={disabled} maxLength={200} autoComplete="organization" />
      </FormField>
      <FormField field="display_name" errors={errors} idPrefix={idPrefix} required helper="Shown as your provider workspace name after setup.">
        <input {...inputA11y('display_name', errors, idPrefix, true)} className={inputClass} value={values.display_name} onChange={event => onChange('display_name', event.target.value)} disabled={disabled} maxLength={100} />
      </FormField>
    </div>
  );
}

export function ContactFields({ values, errors, onChange, disabled = false, idPrefix = 'provider' }: FieldsProps) {
  return (
    <div className="grid grid-cols-1 gap-5 md:grid-cols-2">
      <FormField field="contact_name" errors={errors} idPrefix={idPrefix} required>
        <input {...inputA11y('contact_name', errors, idPrefix)} className={inputClass} value={values.contact_name} onChange={event => onChange('contact_name', event.target.value)} disabled={disabled} maxLength={150} autoComplete="name" />
      </FormField>
      <FormField field="business_phone" errors={errors} idPrefix={idPrefix} required>
        <input {...inputA11y('business_phone', errors, idPrefix)} className={inputClass} type="tel" inputMode="tel" value={values.business_phone} onChange={event => onChange('business_phone', event.target.value)} disabled={disabled} maxLength={32} autoComplete="tel" placeholder="+1 415 555 0123" />
      </FormField>
      <FormField field="support_email" errors={errors} idPrefix={idPrefix} required>
        <input {...inputA11y('support_email', errors, idPrefix)} className={inputClass} type="email" value={values.support_email} onChange={event => onChange('support_email', event.target.value)} disabled={disabled} maxLength={254} autoComplete="email" />
      </FormField>
      <FormField field="billing_email" errors={errors} idPrefix={idPrefix} required>
        <input {...inputA11y('billing_email', errors, idPrefix)} className={inputClass} type="email" value={values.billing_email} onChange={event => onChange('billing_email', event.target.value)} disabled={disabled} maxLength={254} />
      </FormField>
    </div>
  );
}

export function RegionalFields({ values, errors, onChange, disabled = false, idPrefix = 'provider' }: FieldsProps) {
  return (
    <div className="grid grid-cols-1 gap-5 md:grid-cols-2">
      <div className="md:col-span-2">
        <FormField field="website_url" errors={errors} idPrefix={idPrefix} helper="Use an absolute HTTPS URL for production environments.">
          <input {...inputA11y('website_url', errors, idPrefix, true)} className={inputClass} type="url" value={values.website_url} onChange={event => onChange('website_url', event.target.value)} disabled={disabled} maxLength={2048} autoComplete="url" placeholder="https://example.com" />
        </FormField>
      </div>
      <div className="md:col-span-2">
        <FormField field="address_line1" errors={errors} idPrefix={idPrefix} required>
          <input {...inputA11y('address_line1', errors, idPrefix)} className={inputClass} value={values.address_line1} onChange={event => onChange('address_line1', event.target.value)} disabled={disabled} maxLength={200} autoComplete="address-line1" />
        </FormField>
      </div>
      <div className="md:col-span-2">
        <FormField field="address_line2" errors={errors} idPrefix={idPrefix}>
          <input {...inputA11y('address_line2', errors, idPrefix)} className={inputClass} value={values.address_line2} onChange={event => onChange('address_line2', event.target.value)} disabled={disabled} maxLength={200} autoComplete="address-line2" />
        </FormField>
      </div>
      <FormField field="city" errors={errors} idPrefix={idPrefix} required>
        <input {...inputA11y('city', errors, idPrefix)} className={inputClass} value={values.city} onChange={event => onChange('city', event.target.value)} disabled={disabled} maxLength={100} autoComplete="address-level2" />
      </FormField>
      <FormField field="state_province" errors={errors} idPrefix={idPrefix}>
        <input {...inputA11y('state_province', errors, idPrefix)} className={inputClass} value={values.state_province} onChange={event => onChange('state_province', event.target.value)} disabled={disabled} maxLength={100} autoComplete="address-level1" />
      </FormField>
      <FormField field="postal_code" errors={errors} idPrefix={idPrefix} required>
        <input {...inputA11y('postal_code', errors, idPrefix)} className={inputClass} value={values.postal_code} onChange={event => onChange('postal_code', event.target.value)} disabled={disabled} maxLength={32} autoComplete="postal-code" />
      </FormField>
      <FormField field="country_code" errors={errors} idPrefix={idPrefix} required>
        <select {...inputA11y('country_code', errors, idPrefix)} className={inputClass} value={values.country_code} onChange={event => onChange('country_code', event.target.value)} disabled={disabled} autoComplete="country">
          {COUNTRY_CODES.map(code => <option key={code} value={code}>{countryOptionLabel(code)}</option>)}
        </select>
      </FormField>
      <FormField field="default_timezone" errors={errors} idPrefix={idPrefix} required>
        <select {...inputA11y('default_timezone', errors, idPrefix)} className={inputClass} value={values.default_timezone} onChange={event => onChange('default_timezone', event.target.value)} disabled={disabled}>
          {values.default_timezone && !TIMEZONE_OPTIONS.includes(values.default_timezone) && <option value={values.default_timezone}>{values.default_timezone}</option>}
          {TIMEZONE_OPTIONS.map(timezone => <option key={timezone} value={timezone}>{timezone}</option>)}
        </select>
      </FormField>
      <FormField field="default_locale" errors={errors} idPrefix={idPrefix} required>
        <select {...inputA11y('default_locale', errors, idPrefix)} className={inputClass} value={values.default_locale} onChange={event => onChange('default_locale', event.target.value)} disabled={disabled}>
          {values.default_locale && !LOCALE_OPTIONS.includes(values.default_locale) && <option value={values.default_locale}>{values.default_locale}</option>}
          {LOCALE_OPTIONS.map(locale => <option key={locale} value={locale}>{locale}</option>)}
        </select>
      </FormField>
      <FormField field="default_currency" errors={errors} idPrefix={idPrefix} required>
        <select {...inputA11y('default_currency', errors, idPrefix)} className={inputClass} value={values.default_currency} onChange={event => onChange('default_currency', event.target.value)} disabled={disabled}>
          {CURRENCY_CODES.map(code => <option key={code} value={code}>{code}</option>)}
        </select>
      </FormField>
      <FormField field="tax_identifier" errors={errors} idPrefix={idPrefix} helper="Optional. Stored as business profile data and never used as a credential.">
        <input {...inputA11y('tax_identifier', errors, idPrefix, true)} className={inputClass} value={values.tax_identifier} onChange={event => onChange('tax_identifier', event.target.value)} disabled={disabled} maxLength={100} autoComplete="off" />
      </FormField>
    </div>
  );
}

export function ProfileErrorSummary({
  title, errors, idPrefix = 'provider', message,
}: {
  title: string;
  errors?: ProviderProfileErrors;
  idPrefix?: string;
  message?: string;
}) {
  const fields = errors ? Object.keys(errors) as ProviderProfileField[] : [];
  if (!message && fields.length === 0) return null;
  return (
    <div role="alert" aria-live="assertive" tabIndex={-1} className="rounded-lg border border-red-300 bg-red-50 p-4 text-red-900 outline-none dark:border-red-900 dark:bg-red-950/40 dark:text-red-200">
      <h3 className="font-semibold">{title}</h3>
      {message && <p className="mt-1 text-sm">{message}</p>}
      {fields.length > 0 && (
        <ul className="mt-2 list-disc space-y-1 pl-5 text-sm">
          {fields.map(field => (
            <li key={field}><a className="underline hover:no-underline" href={`#${idPrefix}-${field}`}>{errors?.[field]}</a></li>
          ))}
        </ul>
      )}
    </div>
  );
}
