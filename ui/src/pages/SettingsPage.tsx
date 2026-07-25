import { useState, useEffect } from 'react';
import { api } from '@/api/client';
import { useAuth } from '@/hooks/useAuth';

export default function SettingsPage() {
  const { user } = useAuth();
  const [mfaStatus, setMfaStatus] = useState<{ enabled: boolean } | null>(null);
  const [enrollResult, setEnrollResult] = useState<{ secret: string; provisioning_uri: string; qr_code_url: string } | null>(null);
  const [verifyCode, setVerifyCode] = useState('');
  const [mfaMessage, setMfaMessage] = useState('');

  useEffect(() => {
    if (!user?.user_id) return;
    fetch(`/api/v1/mfa/status/${user.user_id}`)
      .then(r => r.json())
      .then(setMfaStatus)
      .catch(() => {});
  }, [user]);

  const handleEnroll = async () => {
    if (!user?.user_id) return;
    try {
      const res = await fetch(`/api/v1/mfa/enroll/${user.user_id}`);
      const data = await res.json();
      setEnrollResult(data);
    } catch {
      setMfaMessage('Failed to enroll MFA');
    }
  };

  const handleVerify = async () => {
    if (!user?.user_id) return;
    try {
      const res = await fetch(`/api/v1/mfa/verify/${user.user_id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code: verifyCode }),
      });
      const data = await res.json();
      if (data.verified) {
        setMfaStatus({ enabled: true });
        setEnrollResult(null);
        setVerifyCode('');
        setMfaMessage('MFA enabled successfully');
      } else {
        setMfaMessage(data.error || 'Invalid code');
      }
    } catch {
      setMfaMessage('Failed to verify code');
    }
  };

  const handleDisable = async () => {
    if (!user?.user_id) return;
    try {
      await fetch(`/api/v1/mfa/${user.user_id}`, { method: 'DELETE' });
      setMfaStatus({ enabled: false });
      setMfaMessage('MFA disabled');
    } catch {
      setMfaMessage('Failed to disable MFA');
    }
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-slate-900 dark:text-white">My Settings</h1>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Profile */}
        <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
          <h2 className="font-semibold mb-3">Profile</h2>
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between"><dt className="text-slate-500">Email</dt><dd>{user?.email}</dd></div>
            <div className="flex justify-between"><dt className="text-slate-500">Role</dt><dd className="capitalize">{user?.role}</dd></div>
          </dl>
        </div>

        {/* MFA */}
        <div className="bg-white dark:bg-slate-900 p-4 rounded-lg border border-slate-200 dark:border-slate-700">
          <h2 className="font-semibold mb-3">Two-Factor Authentication</h2>
          <p className="text-sm text-slate-500 mb-3">
            Status: {mfaStatus === null ? 'Loading...' : mfaStatus.enabled ? 'Enabled' : 'Not configured'}
          </p>

          {!mfaStatus?.enabled && !enrollResult && (
            <button onClick={handleEnroll} className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded hover:bg-blue-700">
              Set Up MFA
            </button>
          )}

          {enrollResult && (
            <div className="space-y-3">
              <div className="flex justify-center">
                <img src={enrollResult.qr_code_url} alt="MFA QR Code" className="w-40 h-40 border rounded" />
              </div>
              <p className="text-xs text-slate-500 text-center">Scan with authenticator app</p>
              <div>
                <label className="block text-xs text-slate-500 mb-1">Or enter this key manually:</label>
                <code className="block text-xs bg-slate-100 dark:bg-slate-800 p-2 rounded break-all">{enrollResult.secret}</code>
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Verify Code</label>
                <div className="flex gap-2">
                  <input
                    type="text" value={verifyCode} onChange={e => setVerifyCode(e.target.value)}
                    className="flex-1 px-3 py-2 border rounded-md dark:bg-slate-800 text-center font-mono text-lg tracking-widest"
                    placeholder="000000" maxLength={6}
                  />
                  <button onClick={handleVerify} className="px-4 py-2 bg-green-600 text-white text-sm rounded hover:bg-green-700">
                    Verify
                  </button>
                </div>
              </div>
            </div>
          )}

          {mfaStatus?.enabled && (
            <button onClick={handleDisable} className="px-3 py-1.5 bg-red-600 text-white text-sm rounded hover:bg-red-700">
              Disable MFA
            </button>
          )}

          {mfaMessage && (
            <p className={`mt-2 text-sm ${mfaMessage.includes('success') || mfaMessage.includes('enabled') ? 'text-green-600' : 'text-red-500'}`}>
              {mfaMessage}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
