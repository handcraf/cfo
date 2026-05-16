import React, { useState } from 'react';
import { setupCompany } from '../api.js';

function Setup({ onComplete }) {
  const [formData, setFormData] = useState({
    name: '',
    industry: '',
    fiscal_year_end: 'December',
    currency: 'USD',
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  async function handleSubmit(e) {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      await setupCompany(formData);
      onComplete();
    } catch (err) {
      setError(err.message || 'Failed to setup company');
    } finally {
      setLoading(false);
    }
  }

  function handleChange(e) {
    const { name, value } = e.target;
    setFormData(prev => ({ ...prev, [name]: value }));
  }

  return (
    <div style={styles.container}>
      <div style={styles.card}>
        <div style={styles.header}>
          <div style={styles.logo}>
            <span style={styles.logoIcon}>◈</span>
            <span style={styles.logoText}>AI CFO</span>
          </div>
          <h1 style={styles.title}>Welcome to AI CFO</h1>
          <p style={styles.subtitle}>
            Your intelligent financial advisor. Let's get started by setting up your company.
          </p>
        </div>

        <form onSubmit={handleSubmit} style={styles.form}>
          <div style={styles.field}>
            <label style={styles.label}>Company Name *</label>
            <input
              type="text"
              name="name"
              value={formData.name}
              onChange={handleChange}
              style={styles.input}
              placeholder="Enter your company name"
              required
            />
          </div>

          <div style={styles.field}>
            <label style={styles.label}>Industry</label>
            <select
              name="industry"
              value={formData.industry}
              onChange={handleChange}
              style={styles.select}
            >
              <option value="">Select industry...</option>
              <option value="Technology">Technology</option>
              <option value="Healthcare">Healthcare</option>
              <option value="Finance">Finance</option>
              <option value="Retail">Retail</option>
              <option value="Manufacturing">Manufacturing</option>
              <option value="Services">Services</option>
              <option value="Other">Other</option>
            </select>
          </div>

          <div style={styles.row}>
            <div style={styles.field}>
              <label style={styles.label}>Fiscal Year End</label>
              <select
                name="fiscal_year_end"
                value={formData.fiscal_year_end}
                onChange={handleChange}
                style={styles.select}
              >
                {['January', 'February', 'March', 'April', 'May', 'June',
                  'July', 'August', 'September', 'October', 'November', 'December'
                ].map(month => (
                  <option key={month} value={month}>{month}</option>
                ))}
              </select>
            </div>

            <div style={styles.field}>
              <label style={styles.label}>Currency</label>
              <select
                name="currency"
                value={formData.currency}
                onChange={handleChange}
                style={styles.select}
              >
                <option value="USD">USD ($)</option>
                <option value="EUR">EUR (€)</option>
                <option value="GBP">GBP (£)</option>
                <option value="INR">INR (₹)</option>
                <option value="JPY">JPY (¥)</option>
              </select>
            </div>
          </div>

          {error && (
            <div style={styles.error}>
              {error}
            </div>
          )}

          <button
            type="submit"
            style={styles.button}
            disabled={loading || !formData.name}
          >
            {loading ? 'Setting up...' : 'Continue to Dashboard →'}
          </button>
        </form>

        <p style={styles.note}>
          You can upload financial documents after setup.
        </p>
      </div>
    </div>
  );
}

const styles = {
  container: {
    minHeight: '100vh',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '40px 20px',
  },
  card: {
    background: 'linear-gradient(145deg, rgba(26, 32, 53, 0.9) 0%, rgba(20, 26, 43, 0.95) 100%)',
    borderRadius: '24px',
    padding: '48px',
    maxWidth: '520px',
    width: '100%',
    border: '1px solid rgba(56, 178, 172, 0.15)',
    boxShadow: '0 25px 80px rgba(0, 0, 0, 0.4), 0 0 40px rgba(56, 178, 172, 0.05)',
  },
  header: {
    textAlign: 'center',
    marginBottom: '40px',
  },
  logo: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '12px',
    marginBottom: '24px',
  },
  logoIcon: {
    fontSize: '32px',
    color: '#38b2ac',
    textShadow: '0 0 20px rgba(56, 178, 172, 0.5)',
  },
  logoText: {
    fontSize: '24px',
    fontWeight: '600',
    background: 'linear-gradient(135deg, #38b2ac 0%, #4fd1c5 100%)',
    WebkitBackgroundClip: 'text',
    WebkitTextFillColor: 'transparent',
    letterSpacing: '-0.5px',
  },
  title: {
    fontSize: '28px',
    fontWeight: '600',
    color: '#f7fafc',
    marginBottom: '12px',
    letterSpacing: '-0.5px',
  },
  subtitle: {
    fontSize: '15px',
    color: '#a0aec0',
    lineHeight: '1.6',
  },
  form: {
    display: 'flex',
    flexDirection: 'column',
    gap: '24px',
  },
  field: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    flex: 1,
  },
  row: {
    display: 'flex',
    gap: '16px',
  },
  label: {
    fontSize: '13px',
    fontWeight: '500',
    color: '#a0aec0',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  input: {
    padding: '14px 16px',
    fontSize: '15px',
    borderRadius: '12px',
    border: '1px solid rgba(74, 85, 104, 0.4)',
    background: 'rgba(13, 17, 28, 0.6)',
    color: '#f7fafc',
    outline: 'none',
    transition: 'all 0.2s ease',
  },
  select: {
    padding: '14px 16px',
    fontSize: '15px',
    borderRadius: '12px',
    border: '1px solid rgba(74, 85, 104, 0.4)',
    background: 'rgba(13, 17, 28, 0.6)',
    color: '#f7fafc',
    outline: 'none',
    cursor: 'pointer',
    appearance: 'none',
    backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' fill='none' viewBox='0 0 24 24' stroke='%23a0aec0'%3E%3Cpath stroke-linecap='round' stroke-linejoin='round' stroke-width='2' d='M19 9l-7 7-7-7'/%3E%3C/svg%3E")`,
    backgroundRepeat: 'no-repeat',
    backgroundPosition: 'right 12px center',
    backgroundSize: '20px',
    paddingRight: '44px',
  },
  error: {
    padding: '12px 16px',
    background: 'rgba(245, 101, 101, 0.15)',
    border: '1px solid rgba(245, 101, 101, 0.3)',
    borderRadius: '10px',
    color: '#fc8181',
    fontSize: '14px',
  },
  button: {
    padding: '16px 24px',
    fontSize: '15px',
    fontWeight: '600',
    borderRadius: '12px',
    border: 'none',
    background: 'linear-gradient(135deg, #38b2ac 0%, #319795 100%)',
    color: '#fff',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
    marginTop: '8px',
  },
  note: {
    textAlign: 'center',
    fontSize: '13px',
    color: '#718096',
    marginTop: '24px',
  },
};

export default Setup;

