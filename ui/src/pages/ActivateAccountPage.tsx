import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { CheckCircle2, Clock3, KeyRound, Mail, ShieldCheck, TriangleAlert } from 'lucide-react';
import { ApiError, api } from '@/api/client';
import type { InvitationInspection } from '@/api/types';
import { ProductAttribution } from '@/components/layout/ProductAttribution';
import { ThemeToggle } from '@/components/shared/ThemeToggle';

type PasswordErrors = {
  password?: string;
  confirmation?: string;
};

type InvitationState =
  | { status: 'loading' }
  | { status: 'unavailable'; message: string }
  | { status: 'ready'; invitation: InvitationInspection }
  | { status: 'accepted' };

const invalidInvitationMessage = 'This invitation is invalid, expired, or no longer available. Ask your provider administrator for a new invitation.';

function validatePasswords(password: string, confirmation: string): PasswordErrors {
  const errors: PasswordErrors = {};
  const passwordBytes = new TextEncoder().encode(password).length;

  if (!password) {
    errors.password = 'Password is required.';
  } else if (passwordBytes < 14 || passwordBytes > 72) {
    errors.password = 'Password must be between 14 and 72 bytes.';
  } else if (/\p{Cc}/u.test(password)) {
    errors.password = 'Password must not contain control characters.';
  }

  if (!confirmation) {
    errors.confirmation = 'Confirm your password.';
  } else if (confirmation !== password) {
    errors.confirmation = 'Passwords do not match.';
  }

  return errors;
}

function acceptanceErrorMessage(caught: unknown) {
  if (caught instanceof ApiError) {
    if (caught.status === 400) return 'Your password did not meet the account security requirements. Review the guidance and try again.';
    if (caught.status === 404) return invalidInvitationMessage;
    if (caught.status === 409) return 'An account already exists for this owner email. You can sign in or contact your provider administrator.';
  }
  return 'We could not activate your account. Please try again. If the problem continues, contact your provider administrator.';
}

function InvitationUnavailable({ message }: { message: string }) {
  return (
    <div role="alert" className="rounded-lg border border-amber-200 bg-amber-50 p-5 text-amber-950 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-100">
      <div className="flex items-start gap-3">
        <TriangleAlert aria-hidden="true" className="mt-0.5 shrink-0 text-amber-600 dark:text-amber-400" size={20} />
        <div>
          <h2 className="font-semibold">Invitation unavailable</h2>
          <p className="mt-1 text-sm leading-6">{message}</p>
          <Link to="/login" className="mt-3 inline-flex rounded-md text-sm font-semibold text-blue-700 underline-offset-4 hover:underline focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:text-blue-300 dark:focus:ring-offset-slate-900">
            Return to sign in
          </Link>
        </div>
      </div>
    </div>
  );
}

export default function ActivateAccountPage() {
  const [token] = useState(() => window.location.hash.startsWith('#') ? window.location.hash.slice(1) : '');
  const [invitationState, setInvitationState] = useState<InvitationState>({ status: 'loading' });
  const [password, setPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [errors, setErrors] = useState<PasswordErrors>({});
  const [submissionError, setSubmissionError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const errorSummaryRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let active = true;
    if (!token) {
      setInvitationState({
        status: 'unavailable',
        message: 'This activation link does not include a valid invitation. Ask your provider administrator for a new invitation.',
      });
      return () => { active = false; };
    }

    api.inspectInvitation(token)
      .then(invitation => {
        if (!active) return;
        const expiry = Date.parse(invitation.expires_at);
        if (!Number.isFinite(expiry) || expiry <= Date.now()) {
          setInvitationState({
            status: 'unavailable',
            message: 'This invitation has expired. Ask your provider administrator for a new invitation.',
          });
          return;
        }
        setInvitationState({ status: 'ready', invitation });
      })
      .catch((caught: unknown) => {
        if (!active) return;
        const message = caught instanceof ApiError && caught.status >= 500
          ? 'We could not verify this invitation right now. Please try again later.'
          : invalidInvitationMessage;
        setInvitationState({ status: 'unavailable', message });
      });

    return () => { active = false; };
  }, [token]);

  const focusErrorSummary = () => {
    window.requestAnimationFrame(() => errorSummaryRef.current?.focus());
  };

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (invitationState.status !== 'ready' || submitting) return;

    const nextErrors = validatePasswords(password, confirmation);
    setErrors(nextErrors);
    setSubmissionError('');
    if (Object.keys(nextErrors).length > 0) {
      focusErrorSummary();
      return;
    }

    setSubmitting(true);
    try {
      await api.acceptInvitation(token, password);
      window.history.replaceState({}, '', window.location.pathname);
      setInvitationState({ status: 'accepted' });
      setPassword('');
      setConfirmation('');
    } catch (caught) {
      setSubmissionError(acceptanceErrorMessage(caught));
      focusErrorSummary();
    } finally {
      setSubmitting(false);
    }
  };

  const clearFieldError = (field: keyof PasswordErrors) => {
    setErrors(current => {
      if (!current[field]) return current;
      const next = { ...current };
      delete next[field];
      return next;
    });
    setSubmissionError('');
  };

  return (
    <div className="flex min-h-screen flex-col bg-slate-50 text-slate-900 dark:bg-slate-950 dark:text-slate-100">
      <header className="border-b border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="mx-auto flex w-full max-w-5xl items-center justify-between px-4 py-3 sm:px-6">
          <div>
            <p className="font-semibold text-slate-950 dark:text-white">Strata RMM</p>
            <p className="text-xs text-slate-500 dark:text-slate-400">MSP owner activation</p>
          </div>
          <ThemeToggle />
        </div>
      </header>

      <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6">
        <div className="w-full max-w-xl">
          <div className="mb-6 text-center">
            <span className="inline-flex rounded-full bg-blue-100 p-3 text-blue-700 dark:bg-blue-950 dark:text-blue-300">
              <ShieldCheck aria-hidden="true" size={24} />
            </span>
            <h1 className="mt-4 text-3xl font-bold tracking-tight text-slate-950 dark:text-white">Activate your owner account</h1>
            <p className="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-400">Choose a password to finish activating your invited MSP workspace.</p>
          </div>

          <section aria-live="polite" className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900 sm:p-8">
            {invitationState.status === 'loading' && (
              <div role="status" className="py-8 text-center text-sm text-slate-500 dark:text-slate-400">Verifying invitation...</div>
            )}

            {invitationState.status === 'unavailable' && <InvitationUnavailable message={invitationState.message} />}

            {invitationState.status === 'accepted' && (
              <div className="py-3 text-center">
                <CheckCircle2 aria-hidden="true" className="mx-auto text-green-600 dark:text-green-400" size={42} />
                <h2 className="mt-4 text-xl font-semibold text-slate-950 dark:text-white">Account activated</h2>
                <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">You can now sign in with your owner email and new password.</p>
                <Link to="/login" className="mt-6 inline-flex rounded-md bg-blue-600 px-4 py-2 text-sm font-semibold text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-slate-900">
                  Go to sign in
                </Link>
              </div>
            )}

            {invitationState.status === 'ready' && (
              <form noValidate onSubmit={handleSubmit}>
                <div className="rounded-lg bg-slate-50 p-4 dark:bg-slate-950/60">
                  <h2 className="font-semibold text-slate-950 dark:text-white">{invitationState.invitation.msp.name}</h2>
                  <dl className="mt-3 space-y-2 text-sm text-slate-600 dark:text-slate-300">
                    <div className="flex items-start gap-2">
                      <Mail aria-hidden="true" className="mt-0.5 shrink-0 text-slate-400" size={16} />
                      <div><dt className="sr-only">Owner email</dt><dd>{invitationState.invitation.masked_email}</dd></div>
                    </div>
                    <div className="flex items-start gap-2">
                      <Clock3 aria-hidden="true" className="mt-0.5 shrink-0 text-slate-400" size={16} />
                      <div><dt className="sr-only">Invitation expiry</dt><dd>Expires {new Date(invitationState.invitation.expires_at).toLocaleString()}</dd></div>
                    </div>
                  </dl>
                </div>

                {(submissionError || Object.keys(errors).length > 0) && (
                  <div ref={errorSummaryRef} tabIndex={-1} role="alert" className="mt-5 rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-900 outline-none focus:ring-2 focus:ring-red-500 dark:border-red-900 dark:bg-red-950/40 dark:text-red-100">
                    <p className="font-semibold">Account activation could not be completed</p>
                    {submissionError && <p className="mt-1 leading-6">{submissionError}</p>}
                    {Object.keys(errors).length > 0 && (
                      <ul className="mt-2 list-disc space-y-1 pl-5">
                        {errors.password && <li>{errors.password}</li>}
                        {errors.confirmation && <li>{errors.confirmation}</li>}
                      </ul>
                    )}
                  </div>
                )}

                <div className="mt-5 space-y-4">
                  <div>
                    <label htmlFor="activation-password" className="block text-sm font-medium text-slate-700 dark:text-slate-300">New password</label>
                    <input
                      id="activation-password"
                      type="password"
                      autoComplete="new-password"
                      value={password}
                      onChange={event => { setPassword(event.target.value); clearFieldError('password'); }}
                      aria-invalid={Boolean(errors.password)}
                      aria-describedby={errors.password ? 'activation-password-error activation-password-help' : 'activation-password-help'}
                      disabled={submitting}
                      className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-slate-950 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/30 disabled:opacity-60 dark:border-slate-700 dark:bg-slate-950 dark:text-white"
                    />
                    <p id="activation-password-help" className="mt-1.5 text-xs leading-5 text-slate-500 dark:text-slate-400">Use 14–72 bytes. Control characters are not allowed.</p>
                    {errors.password && <p id="activation-password-error" className="mt-1 text-sm text-red-700 dark:text-red-300">{errors.password}</p>}
                  </div>

                  <div>
                    <label htmlFor="activation-confirmation" className="block text-sm font-medium text-slate-700 dark:text-slate-300">Confirm new password</label>
                    <input
                      id="activation-confirmation"
                      type="password"
                      autoComplete="new-password"
                      value={confirmation}
                      onChange={event => { setConfirmation(event.target.value); clearFieldError('confirmation'); }}
                      aria-invalid={Boolean(errors.confirmation)}
                      aria-describedby={errors.confirmation ? 'activation-confirmation-error' : undefined}
                      disabled={submitting}
                      className="mt-1 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-slate-950 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/30 disabled:opacity-60 dark:border-slate-700 dark:bg-slate-950 dark:text-white"
                    />
                    {errors.confirmation && <p id="activation-confirmation-error" className="mt-1 text-sm text-red-700 dark:text-red-300">{errors.confirmation}</p>}
                  </div>
                </div>

                <button type="submit" disabled={submitting} className="mt-6 inline-flex w-full items-center justify-center gap-2 rounded-md bg-blue-600 px-4 py-2.5 text-sm font-semibold text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60 dark:focus:ring-offset-slate-900">
                  <KeyRound aria-hidden="true" size={17} />
                  {submitting ? 'Activating account...' : 'Activate account'}
                </button>
              </form>
            )}
          </section>
        </div>
      </main>

      <footer className="mx-auto w-full max-w-5xl px-4 pb-4 sm:px-6">
        <ProductAttribution collapsed={false} />
      </footer>
    </div>
  );
}
