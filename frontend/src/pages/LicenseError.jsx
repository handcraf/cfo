import React from 'react';

// LicenseError is the dead-end screen shown when the backend reports
// an invalid license. The status payload tells us the precise reason
// AND the recommended action so we can show the right CTA.
export default function LicenseError({ status, onRetry }) {
  const reason = status?.reason || 'unknown';
  const message = status?.message || 'License validation failed.';
  const days = status?.days_remaining;
  const machine = status?.machine_id;
  const action = pickAction(reason);

  return (
    <div style={s.wrap}>
      <div style={s.card}>
        <div style={s.icon}>!</div>
        <div style={s.title}>License Required</div>
        <div style={s.subtitle}>AI CFO cannot start until this is resolved.</div>

        <div style={s.box}>
          <Row label="Status">{message}</Row>
          <Row label="Code"><code style={s.code}>{reason}</code></Row>
          {typeof days === 'number' && days <= 0 && (
            <Row label="Expired">{Math.abs(days)} days ago</Row>
          )}
          {machine && <Row label="Machine ID"><code style={s.code}>{shortHash(machine)}</code></Row>}
        </div>

        <div style={s.actionBlock}>
          <div style={s.actionTitle}>Next steps</div>
          <ol style={s.steps}>
            {action.steps.map((step, i) => <li key={i} style={s.step}>{step}</li>)}
          </ol>
        </div>

        <button onClick={onRetry} style={s.button}>I've fixed it — retry</button>

        <div style={s.footer}>
          Contact your vendor with the Machine ID above when requesting a new license.
        </div>
      </div>
    </div>
  );
}

function Row({ label, children }) {
  return (
    <div style={s.row}>
      <span style={s.rowLabel}>{label}</span>
      <span style={s.rowValue}>{children}</span>
    </div>
  );
}

function shortHash(s) {
  if (!s || s.length < 16) return s;
  return s.slice(0, 8) + '…' + s.slice(-8);
}

function pickAction(reason) {
  switch (reason) {
    case 'file_missing':
      return {
        steps: [
          'Place your license.lic file in the AI CFO install directory.',
          'Restart the service (./run.sh stop && ./run.sh start).',
        ],
      };
    case 'expired':
      return {
        steps: [
          'Run `cfo-license export-request` to generate request.dat',
          'Send request.dat to your vendor.',
          'Replace license.lic with the renewed file you receive back.',
          'Restart the service.',
        ],
      };
    case 'machine_mismatch':
      return {
        steps: [
          'On the OLD machine, run `cfo-license deactivate` to generate migration.dat.',
          'Copy license.lic, migration.dat, and migration_pub.key to this machine.',
          'Run `cfo-license activate migration.dat`.',
          'Restart the service.',
        ],
      };
    case 'bad_signature':
    case 'bad_format':
      return {
        steps: [
          'Your license file appears to be corrupted or tampered with.',
          'Do not edit license.lic manually — any change invalidates the signature.',
          'Contact your vendor for a fresh copy.',
        ],
      };
    case 'no_public_key':
      return {
        steps: [
          'This build of AI CFO has no embedded vendor public key.',
          'This is a deployment misconfiguration — contact support.',
        ],
      };
    default:
      return {
        steps: [
          'Run `cfo-license status` on the server for more detail.',
          'Contact your vendor with the Machine ID shown above.',
        ],
      };
  }
}

const s = {
  wrap: { minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '24px' },
  card: {
    background: 'rgba(255,255,255,0.04)', borderRadius: '16px', padding: '40px',
    width: '100%', maxWidth: '560px', border: '1px solid rgba(245,101,101,0.3)',
    boxShadow: '0 20px 60px rgba(0,0,0,0.4)',
  },
  icon: {
    width: '56px', height: '56px', borderRadius: '50%',
    background: 'linear-gradient(135deg, #f56565, #c53030)',
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    fontSize: '32px', fontWeight: 700, color: '#fff', margin: '0 auto 16px',
  },
  title: { fontSize: '24px', fontWeight: 700, textAlign: 'center' },
  subtitle: { fontSize: '14px', color: '#a0aec0', textAlign: 'center', marginTop: '8px', marginBottom: '28px' },
  box: {
    background: 'rgba(0,0,0,0.3)', borderRadius: '8px',
    border: '1px solid rgba(255,255,255,0.08)', padding: '16px', marginBottom: '20px',
  },
  row: { display: 'flex', justifyContent: 'space-between', padding: '6px 0', fontSize: '13px' },
  rowLabel: { color: '#a0aec0' },
  rowValue: { color: '#e4e8ef', textAlign: 'right', maxWidth: '70%' },
  code: { background: 'rgba(255,255,255,0.06)', padding: '2px 6px', borderRadius: '4px', fontSize: '12px' },
  actionBlock: { marginTop: '16px', marginBottom: '20px' },
  actionTitle: { fontSize: '12px', color: '#a0aec0', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '10px' },
  steps: { paddingLeft: '20px', color: '#cbd5e0', fontSize: '14px', lineHeight: 1.7 },
  step: { marginBottom: '4px' },
  button: {
    width: '100%', padding: '12px', borderRadius: '8px', border: 'none',
    background: 'linear-gradient(135deg, #38b2ac, #319795)', color: '#fff',
    fontSize: '14px', fontWeight: 600, cursor: 'pointer',
  },
  footer: { marginTop: '24px', fontSize: '11px', color: '#718096', textAlign: 'center' },
};
