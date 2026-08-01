import type { ProviderBusinessProfileValues } from '@/api/types';

export type ProviderProfileField = keyof ProviderBusinessProfileValues;
export type ProviderProfileErrors = Partial<Record<ProviderProfileField, string>>;

export const EMPTY_PROVIDER_PROFILE: ProviderBusinessProfileValues = {
  legal_name: '',
  display_name: '',
  contact_name: '',
  support_email: '',
  billing_email: '',
  business_phone: '',
  website_url: '',
  address_line1: '',
  address_line2: '',
  city: '',
  state_province: '',
  postal_code: '',
  country_code: 'US',
  default_timezone: 'UTC',
  default_locale: 'en-US',
  default_currency: 'USD',
  tax_identifier: '',
};

export const BUSINESS_FIELDS: ProviderProfileField[] = ['legal_name', 'display_name'];
export const CONTACT_FIELDS: ProviderProfileField[] = ['contact_name', 'support_email', 'billing_email', 'business_phone'];
export const REGIONAL_FIELDS: ProviderProfileField[] = [
  'website_url', 'address_line1', 'address_line2', 'city', 'state_province', 'postal_code',
  'country_code', 'default_timezone', 'default_locale', 'default_currency', 'tax_identifier',
];
export const ALL_PROVIDER_FIELDS: ProviderProfileField[] = [...BUSINESS_FIELDS, ...CONTACT_FIELDS, ...REGIONAL_FIELDS];

export const PROVIDER_FIELD_LABELS: Record<ProviderProfileField, string> = {
  legal_name: 'Legal business name',
  display_name: 'Display name',
  contact_name: 'Primary contact name',
  support_email: 'Support email',
  billing_email: 'Billing email',
  business_phone: 'Business phone',
  website_url: 'Website URL',
  address_line1: 'Address line 1',
  address_line2: 'Address line 2',
  city: 'City',
  state_province: 'State or province',
  postal_code: 'Postal code',
  country_code: 'Country',
  default_timezone: 'Default timezone',
  default_locale: 'Default locale',
  default_currency: 'Default currency',
  tax_identifier: 'Tax identifier',
};

const REQUIRED_FIELDS = new Set<ProviderProfileField>([
  'legal_name', 'display_name', 'contact_name', 'support_email', 'billing_email',
  'business_phone', 'address_line1', 'city', 'postal_code', 'country_code',
  'default_timezone', 'default_locale', 'default_currency',
]);

const MAX_LENGTHS: Partial<Record<ProviderProfileField, number>> = {
  legal_name: 200,
  display_name: 100,
  contact_name: 150,
  support_email: 254,
  billing_email: 254,
  business_phone: 32,
  website_url: 2048,
  address_line1: 200,
  address_line2: 200,
  city: 100,
  state_province: 100,
  postal_code: 32,
  default_timezone: 64,
  tax_identifier: 100,
};

export const COUNTRY_CODES = `
  AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ
  CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR
  GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO
  JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR
  MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO
  RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV
  TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW
`.trim().split(/\s+/);

export const CURRENCY_CODES = `
  AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYN BZD CAD CDF
  CHE CHF CHW CLF CLP CNY COP COU CRC CUC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD
  GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP
  LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB
  PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SLL SOS SRD SSP STN SVC SYP SZL THB
  TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD USN UYI UYU UYW UZS VED VES VND VUV WST XAF XAG XAU XBA XBB XBC
  XBD XCD XDR XOF XPD XPF XPT XSU XUA YER ZAR ZMW ZWG
`.trim().split(/\s+/);

const timezoneFallback = [
  'UTC', 'Africa/Cairo', 'Africa/Johannesburg', 'America/Anchorage', 'America/Chicago',
  'America/Denver', 'America/Halifax', 'America/Los_Angeles', 'America/Mexico_City',
  'America/New_York', 'America/Phoenix', 'America/Sao_Paulo', 'Asia/Dubai', 'Asia/Hong_Kong',
  'Asia/Kolkata', 'Asia/Seoul', 'Asia/Shanghai', 'Asia/Singapore', 'Asia/Tokyo',
  'Australia/Perth', 'Australia/Sydney', 'Europe/Amsterdam', 'Europe/Berlin', 'Europe/London',
  'Europe/Madrid', 'Europe/Paris', 'Europe/Rome', 'Pacific/Auckland', 'Pacific/Honolulu',
];

type ExtendedIntl = typeof Intl & { supportedValuesOf?: (key: 'timeZone') => string[] };
const browserTimezones = (Intl as ExtendedIntl).supportedValuesOf?.('timeZone') ?? timezoneFallback;
export const TIMEZONE_OPTIONS = Array.from(new Set(['UTC', ...browserTimezones])).sort();

export const LOCALE_OPTIONS = [
  'ar-AE', 'de-DE', 'en-AU', 'en-CA', 'en-GB', 'en-US', 'es-ES', 'es-MX',
  'fr-CA', 'fr-FR', 'hi-IN', 'it-IT', 'ja-JP', 'ko-KR', 'nl-NL', 'pl-PL',
  'pt-BR', 'pt-PT', 'sv-SE', 'zh-CN', 'zh-TW',
];

const countryNames: Record<string, string> = {
  AU: 'Australia', CA: 'Canada', DE: 'Germany', ES: 'Spain', FR: 'France', GB: 'United Kingdom',
  IN: 'India', JP: 'Japan', MX: 'Mexico', NL: 'Netherlands', NZ: 'New Zealand', SG: 'Singapore',
  US: 'United States', ZA: 'South Africa',
};

export function countryOptionLabel(code: string) {
  return countryNames[code] ? `${countryNames[code]} (${code})` : code;
}

function validTimezone(value: string) {
  if (!value || value === 'Local') return false;
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: value }).format();
    return true;
  } catch {
    return false;
  }
}

function runeLength(value: string) {
  return Array.from(value).length;
}

function validateText(field: ProviderProfileField, value: string) {
  const label = PROVIDER_FIELD_LABELS[field];
  const trimmed = value.trim();
  if (REQUIRED_FIELDS.has(field) && !trimmed) return `${label} is required.`;
  const max = MAX_LENGTHS[field];
  if (max && runeLength(trimmed) > max) return `${label} must be ${max} characters or fewer.`;
  if (/\p{Cc}/u.test(trimmed)) return `${label} must not contain control characters.`;
  return '';
}

function validEmail(value: string) {
  return /^[A-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?(?:\.[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?)+$/i.test(value);
}

export function validateProviderProfile(
  values: ProviderBusinessProfileValues,
  fields: ProviderProfileField[] = ALL_PROVIDER_FIELDS,
  requireHTTPS = import.meta.env.PROD,
) {
  const errors: ProviderProfileErrors = {};
  const selectedFields = new Set(fields);

  for (const field of fields) {
    const message = validateText(field, values[field]);
    if (message) errors[field] = message;
  }

  for (const field of ['support_email', 'billing_email'] as const) {
    if (selectedFields.has(field) && !errors[field] && !validEmail(values[field].trim())) {
      errors[field] = `${PROVIDER_FIELD_LABELS[field]} must be a valid email address.`;
    }
  }

  if (selectedFields.has('business_phone') && !errors.business_phone) {
    const phone = values.business_phone.trim();
    const digits = phone.replace(/\D/g, '').length;
    if (!/^[+0-9][0-9 ().xX+-]*$/.test(phone) || digits < 7) {
      errors.business_phone = 'Business phone must be a valid phone number with at least 7 digits.';
    }
  }

  if (selectedFields.has('website_url') && !errors.website_url && values.website_url.trim()) {
    try {
      const parsed = new URL(values.website_url.trim());
      const productionProtocolValid = !requireHTTPS || parsed.protocol === 'https:';
      if (!['http:', 'https:'].includes(parsed.protocol) || !parsed.host || parsed.username || parsed.password || !productionProtocolValid) {
        errors.website_url = requireHTTPS
          ? 'Website URL must be an absolute HTTPS URL without credentials.'
          : 'Website URL must be an absolute HTTP(S) URL without credentials.';
      } else {
        const decodedPath = decodeURIComponent(parsed.pathname);
        const decodedQuery = decodeURIComponent(parsed.search.slice(1).replace(/\+/g, ' '));
        if (/\p{Cc}/u.test(decodedPath) || /\p{Cc}/u.test(decodedQuery)) {
          errors.website_url = 'Website URL contains invalid escaped characters.';
        }
      }
    } catch {
      errors.website_url = 'Website URL must be an absolute HTTP(S) URL without credentials.';
    }
  }

  if (selectedFields.has('country_code') && !errors.country_code && !COUNTRY_CODES.includes(values.country_code.toUpperCase())) {
    errors.country_code = 'Country must be a supported ISO 3166-1 country.';
  }
  if (selectedFields.has('default_currency') && !errors.default_currency && !CURRENCY_CODES.includes(values.default_currency.toUpperCase())) {
    errors.default_currency = 'Default currency must be a supported ISO 4217 currency.';
  }
  if (selectedFields.has('default_timezone') && !errors.default_timezone && !validTimezone(values.default_timezone)) {
    errors.default_timezone = 'Default timezone must be a valid IANA timezone.';
  }
  if (selectedFields.has('default_locale') && !errors.default_locale && !/^[A-Za-z]{2,3}(?:-[A-Za-z]{2}|-[0-9]{3})?$/.test(values.default_locale.trim())) {
    errors.default_locale = 'Default locale must be a valid language or language-region tag.';
  }
  return errors;
}

export function normalizeProviderProfile(values: ProviderBusinessProfileValues): ProviderBusinessProfileValues {
  const normalized = Object.fromEntries(
    Object.entries(values).map(([key, value]) => [key, value.trim()]),
  ) as ProviderBusinessProfileValues;
  normalized.support_email = normalized.support_email.toLowerCase();
  normalized.billing_email = normalized.billing_email.toLowerCase();
  normalized.country_code = normalized.country_code.toUpperCase();
  normalized.default_currency = normalized.default_currency.toUpperCase();
  const localeParts = normalized.default_locale.split('-');
  localeParts[0] = localeParts[0].toLowerCase();
  if (localeParts[1]?.length === 2) localeParts[1] = localeParts[1].toUpperCase();
  normalized.default_locale = localeParts.join('-');
  return normalized;
}

export function firstErrorStep(errors: ProviderProfileErrors) {
  const fields = Object.keys(errors) as ProviderProfileField[];
  if (fields.some(field => BUSINESS_FIELDS.includes(field))) return 0;
  if (fields.some(field => CONTACT_FIELDS.includes(field))) return 1;
  return 2;
}
