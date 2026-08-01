import { describe, expect, it } from 'vitest';
import { EMPTY_PROVIDER_PROFILE, validateProviderProfile } from '@/lib/providerProfile';

const validProfile = {
  ...EMPTY_PROVIDER_PROFILE,
  legal_name: 'Example Provider LLC',
  display_name: 'Example Provider',
  contact_name: 'Ada Admin',
  support_email: 'support@example.test',
  billing_email: 'billing@example.test',
  business_phone: '+1 415 555 0123',
  website_url: 'https://example.test',
  address_line1: '100 Market Street',
  city: 'San Francisco',
  postal_code: '94105',
};

describe('provider profile validation', () => {
  it('accepts a complete profile matching the backend contract', () => {
    expect(validateProviderProfile(validProfile, undefined, true)).toEqual({});
  });

  it('matches backend format and production URL constraints', () => {
    const errors = validateProviderProfile({
      ...validProfile,
      support_email: 'not-an-email',
      billing_email: 'also-invalid',
      business_phone: '123',
      website_url: 'http://example.test',
      country_code: 'ZZ',
      default_timezone: 'Local',
      default_locale: 'english_US',
      default_currency: 'ZZZ',
    }, undefined, true);

    expect(errors.support_email).toMatch(/valid email/);
    expect(errors.billing_email).toMatch(/valid email/);
    expect(errors.business_phone).toMatch(/at least 7 digits/);
    expect(errors.website_url).toMatch(/absolute HTTPS URL/);
    expect(errors.country_code).toMatch(/ISO 3166-1/);
    expect(errors.default_timezone).toMatch(/IANA timezone/);
    expect(errors.default_locale).toMatch(/language or language-region/);
    expect(errors.default_currency).toMatch(/ISO 4217/);
  });
});
