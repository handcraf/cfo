import React, { useState } from 'react';
import { authSetup, authLogin } from '../api.js';

// LoginPage handles both first-time setup (no password yet) and the
// normal login flow. The parent gives us `needsSetup` from the
// /auth/status payload.
export default function LoginPage({ needsSetup, onSuccess }) {
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  async function handleSubmit(e) {
    e.preventDefault();
    setError('');
    if (needsSetup) {
      if (password.length < 6) {
        setError('Password must be at least 6 characters.');
        return;
      }
      if (password !== confirm) {
        setError('Passwords do not match.');
        return;
      }
    }
    setSubmitting(true);
    try {
      if (needsSetup) {
        await authSetup(password);
      } else {
        await authLogin(password);
      }
      onSuccess();
    } catch (err) {
      setError(err.body?.error || err.message || 'Login failed');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div style={s.wrap}>
      <form style={s.card} onSubmit={handleSubmit}>
        <div style={s.logo}>AI CFO</div>
        <div style={s.subtitle}>
          {needsSetup ? 'First-time setup' : 'Sign in to continue'}
        </div>

        {needsSetup && (
          <div style={s.notice}>
            This is the first time you're starting AI CFO on this machine.
            Choose a password — you'll use it every time you open the app.
          </div>
        )}

        <label style={s.label}>Password</label>
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder={needsSetup ? 'minimum 6 characters' : ''}
          style={s.input}
          autoFocus
          autoComplete={needsSetup ? 'new-password' : 'current-password'}
        />

        {needsSetup && (
          <>
            <label style={s.label}>Confirm password</label>
            <input
              type="password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              style={s.input}
              autoComplete="new-password"
            />
          </>
        )}

        {error && <div style={s.error}>{error}</div>}

        <button type="submit" disabled={submitting} style={s.button}>
          {submitting ? '...' : needsSetup ? 'Create password' : 'Sign in'}
        </button>

        <div style={s.footer}>
          Single-tenant, on-premises install · Air-gapped · No telemetry
        </div>
      </form>
    </div>
  );
}

const s = {
  wrap: {
    minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
    padding: '24px',
  },
  card: {
    background: 'rgba(255,255,255,0.04)', borderRadius: '16px', padding: '40px',
    width: '100%', maxWidth: '420px', backdropFilter: 'blur(10px)',
    border: '1px solid rgba(255,255,255,0.08)', boxShadow: '0 20px 60px rgba(0,0,0,0.4)',
  },
  logo: { fontSize: '32px', fontWeight: 700, letterSpacing: '-0.02em', textAlign: 'center' },
  subtitle: { fontSize: '14px', color: '#a0aec0', textAlign: 'center', marginTop: '8px', marginBottom: '32px' },
  notice: {
    background: 'rgba(56,178,172,0.1)', border: '1px solid rgba(56,178,172,0.3)',
    color: '#81e6d9', padding: '12px', borderRadius: '8px', fontSize: '13px',
    marginBottom: '24px', lineHeight: 1.5,
  },
  label: { fontSize: '12px', color: '#a0aec0', display: 'block', marginTop: '16px', marginBottom: '6px',
           textTransform: 'uppercase', letterSpacing: '0.05em' },
  input: {
    width: '100%', padding: '12px 16px', borderRadius: '8px',
    border: '1px solid rgba(255,255,255,0.1)', background: 'rgba(0,0,0,0.3)',
    color: '#e4e8ef', fontSize: '14px', outline: 'none',
  },
  error: {
    background: 'rgba(245,101,101,0.15)', border: '1px solid rgba(245,101,101,0.4)',
    color: '#fc8181', padding: '10px', borderRadius: '8px', fontSize: '13px',
    marginTop: '16px',
  },
  button: {
    width: '100%', padding: '14px', marginTop: '24px', borderRadius: '8px',
    border: 'none', background: 'linear-gradient(135deg, #38b2ac 0%, #319795 100%)',
    color: '#fff', fontSize: '15px', fontWeight: 600, cursor: 'pointer',
  },
  footer: { marginTop: '32px', fontSize: '11px', color: '#718096', textAlign: 'center' },
};
