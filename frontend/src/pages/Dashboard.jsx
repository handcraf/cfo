import React, { useState, useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { getMetrics, getDocuments, resetCompany, resetDocuments } from '../api.js';
import MetricCard from '../components/MetricCard.jsx';
import DocumentTable from '../components/DocumentTable.jsx';
import UploadBox from '../components/UploadBox.jsx';

function Dashboard() {
  const [metrics, setMetrics] = useState(null);
  const [documents, setDocuments] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showUpload, setShowUpload] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    loadData();
  }, []);

  async function loadData() {
    try {
      const [metricsData, docsData] = await Promise.all([
        getMetrics(),
        getDocuments(),
      ]);
      setMetrics(metricsData);
      setDocuments(docsData.documents || []);
    } catch (error) {
      console.error('Failed to load data:', error);
    } finally {
      setLoading(false);
    }
  }

  function handleUploadComplete() {
    setShowUpload(false);
    loadData();
  }

  async function handleSwitchCompany() {
    if (!window.confirm('⚠️ This will delete ALL data (company + documents) and start fresh. Are you sure?')) {
      return;
    }
    try {
      await resetCompany();
      // Force reload to go back to setup
      window.location.href = '/setup';
    } catch (error) {
      console.error('Failed to reset company:', error);
      alert('Failed to reset: ' + error.message);
    }
  }

  async function handleResetDocuments() {
    if (!window.confirm('⚠️ This will delete ALL uploaded documents. Company settings will be kept. Are you sure?')) {
      return;
    }
    try {
      await resetDocuments();
      setShowSettings(false);
      loadData();
    } catch (error) {
      console.error('Failed to reset documents:', error);
      alert('Failed to reset: ' + error.message);
    }
  }

  function formatCurrency(value) {
    if (value === null || value === undefined) return '—';
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(value);
  }

  function formatMonths(value) {
    if (value === null || value === undefined) return '—';
    if (value >= 999) return '∞';
    return `${value.toFixed(1)} mo`;
  }

  function formatPercent(value) {
    if (value === null || value === undefined) return null;
    const sign = value >= 0 ? '+' : '';
    return `${sign}${value.toFixed(1)}%`;
  }

  if (loading) {
    return (
      <div style={styles.loadingContainer}>
        <div style={styles.spinner}></div>
        <p>Loading financial data...</p>
      </div>
    );
  }

  return (
    <div style={styles.container}>
      {/* Header */}
      <header style={styles.header}>
        <div style={styles.headerLeft}>
          <div style={styles.logo}>
            <span style={styles.logoIcon}>◈</span>
            <span style={styles.logoText}>AI CFO</span>
          </div>
        </div>
        <div style={styles.headerRight}>
          <button
            style={styles.settingsBtn}
            onClick={() => setShowSettings(true)}
            title="Settings"
          >
            ⚙️
          </button>
          <button
            style={styles.uploadBtn}
            onClick={() => setShowUpload(true)}
          >
            ↑ Upload Document
          </button>
          <Link to="/ask" style={styles.askBtn}>
            Ask CFO →
          </Link>
        </div>
      </header>

      {/* Main Content */}
      <main style={styles.main}>
        {/* Metrics Grid */}
        <section style={styles.metricsSection}>
          <h2 style={styles.sectionTitle}>Financial Overview</h2>
          <div style={styles.metricsGrid}>
            <MetricCard
              title="Cash Position"
              value={formatCurrency(metrics?.cash)}
              trend={formatPercent(metrics?.trends?.cash_change_pct)}
              icon="💰"
              color="#38b2ac"
            />
            <MetricCard
              title="Monthly Burn"
              value={formatCurrency(metrics?.monthly_burn)}
              trend={formatPercent(metrics?.trends?.burn_change_pct)}
              icon="🔥"
              color="#ed8936"
              invertTrend
            />
            <MetricCard
              title="Runway"
              value={formatMonths(metrics?.runway_months)}
              icon="🛫"
              color="#9f7aea"
            />
            <MetricCard
              title="Revenue"
              value={formatCurrency(metrics?.revenue)}
              trend={formatPercent(metrics?.trends?.revenue_change_pct)}
              icon="📈"
              color="#48bb78"
            />
          </div>
        </section>

        {/* Additional Metrics */}
        {(metrics?.net_income || metrics?.total_assets || metrics?.equity) && (
          <section style={styles.metricsSection}>
            <h2 style={styles.sectionTitle}>Balance Sheet Highlights</h2>
            <div style={styles.metricsGrid}>
              <MetricCard
                title="Net Income"
                value={formatCurrency(metrics?.net_income)}
                icon="💵"
                color="#4299e1"
              />
              <MetricCard
                title="Total Assets"
                value={formatCurrency(metrics?.total_assets)}
                icon="🏢"
                color="#667eea"
              />
              <MetricCard
                title="Total Liabilities"
                value={formatCurrency(metrics?.total_liabilities)}
                icon="📋"
                color="#f56565"
              />
              <MetricCard
                title="Equity"
                value={formatCurrency(metrics?.equity)}
                icon="⚖️"
                color="#38b2ac"
              />
            </div>
          </section>
        )}

        {/* Documents Table */}
        <section style={styles.documentsSection}>
          <div style={styles.sectionHeader}>
            <h2 style={styles.sectionTitle}>Uploaded Documents</h2>
            <span style={styles.docCount}>{documents.length} files</span>
          </div>
          <DocumentTable documents={documents} />
          
          {documents.length === 0 && (
            <div style={styles.emptyState}>
              <p style={styles.emptyText}>No documents uploaded yet.</p>
              <button
                style={styles.emptyBtn}
                onClick={() => setShowUpload(true)}
              >
                Upload your first document
              </button>
            </div>
          )}
        </section>

        {/* Data Sources */}
        {metrics?.data_sources?.length > 0 && (
          <div style={styles.dataSources}>
            <span style={styles.dataSourcesLabel}>Data from:</span>
            {metrics.data_sources.slice(0, 3).map((id, i) => (
              <span key={id} style={styles.dataSourceBadge}>
                {id.substring(0, 12)}...
              </span>
            ))}
            {metrics.data_sources.length > 3 && (
              <span style={styles.dataSourceMore}>
                +{metrics.data_sources.length - 3} more
              </span>
            )}
          </div>
        )}

        {/* Errors/Warnings */}
        {metrics?.errors?.length > 0 && (
          <div style={styles.warnings}>
            {metrics.errors.map((err, i) => (
              <div key={i} style={styles.warning}>
                ⚠️ {err}
              </div>
            ))}
          </div>
        )}
      </main>

      {/* Upload Modal */}
      {showUpload && (
        <div style={styles.modal} onClick={() => setShowUpload(false)}>
          <div style={styles.modalContent} onClick={e => e.stopPropagation()}>
            <button
              style={styles.closeBtn}
              onClick={() => setShowUpload(false)}
            >
              ×
            </button>
            <UploadBox onComplete={handleUploadComplete} />
          </div>
        </div>
      )}

      {/* Settings Modal */}
      {showSettings && (
        <div style={styles.modal} onClick={() => setShowSettings(false)}>
          <div style={styles.settingsModalContent} onClick={e => e.stopPropagation()}>
            <button
              style={styles.closeBtn}
              onClick={() => setShowSettings(false)}
            >
              ×
            </button>
            <h2 style={styles.settingsTitle}>⚙️ Settings</h2>
            
            <div style={styles.settingsSection}>
              <h3 style={styles.settingsSectionTitle}>Data Management</h3>
              
              <div style={styles.settingsItem}>
                <div style={styles.settingsItemInfo}>
                  <span style={styles.settingsItemTitle}>🗑️ Reset Documents</span>
                  <span style={styles.settingsItemDesc}>
                    Delete all uploaded documents but keep company settings
                  </span>
                </div>
                <button
                  style={styles.resetDocsBtn}
                  onClick={handleResetDocuments}
                >
                  Reset Documents
                </button>
              </div>

              <div style={styles.settingsItem}>
                <div style={styles.settingsItemInfo}>
                  <span style={styles.settingsItemTitle}>🔄 Switch Company</span>
                  <span style={styles.settingsItemDesc}>
                    Delete ALL data and onboard a new company
                  </span>
                </div>
                <button
                  style={styles.switchCompanyBtn}
                  onClick={handleSwitchCompany}
                >
                  Switch Company
                </button>
              </div>
            </div>

            <div style={styles.settingsInfo}>
              <p>📁 Documents: {documents.length} files</p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

const styles = {
  container: {
    minHeight: '100vh',
    padding: '0 0 60px 0',
  },
  loadingContainer: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    height: '100vh',
    gap: '20px',
    color: '#a0aec0',
  },
  spinner: {
    width: '40px',
    height: '40px',
    border: '3px solid #2d3748',
    borderTopColor: '#38b2ac',
    borderRadius: '50%',
    animation: 'spin 1s linear infinite',
  },
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '20px 40px',
    borderBottom: '1px solid rgba(74, 85, 104, 0.2)',
  },
  headerLeft: {
    display: 'flex',
    alignItems: 'center',
    gap: '40px',
  },
  logo: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
  },
  logoIcon: {
    fontSize: '24px',
    color: '#38b2ac',
    textShadow: '0 0 15px rgba(56, 178, 172, 0.5)',
  },
  logoText: {
    fontSize: '20px',
    fontWeight: '600',
    background: 'linear-gradient(135deg, #38b2ac 0%, #4fd1c5 100%)',
    WebkitBackgroundClip: 'text',
    WebkitTextFillColor: 'transparent',
  },
  headerRight: {
    display: 'flex',
    gap: '12px',
    alignItems: 'center',
  },
  settingsBtn: {
    width: '40px',
    height: '40px',
    fontSize: '18px',
    borderRadius: '10px',
    border: '1px solid rgba(74, 85, 104, 0.3)',
    background: 'transparent',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    transition: 'all 0.2s ease',
  },
  uploadBtn: {
    padding: '10px 20px',
    fontSize: '14px',
    fontWeight: '500',
    borderRadius: '10px',
    border: '1px solid rgba(56, 178, 172, 0.3)',
    background: 'transparent',
    color: '#38b2ac',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
  },
  askBtn: {
    padding: '10px 24px',
    fontSize: '14px',
    fontWeight: '600',
    borderRadius: '10px',
    border: 'none',
    background: 'linear-gradient(135deg, #38b2ac 0%, #319795 100%)',
    color: '#fff',
    cursor: 'pointer',
    textDecoration: 'none',
    transition: 'all 0.2s ease',
  },
  main: {
    maxWidth: '1200px',
    margin: '0 auto',
    padding: '40px',
  },
  metricsSection: {
    marginBottom: '40px',
  },
  sectionTitle: {
    fontSize: '18px',
    fontWeight: '600',
    color: '#e2e8f0',
    marginBottom: '20px',
    letterSpacing: '-0.3px',
  },
  metricsGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))',
    gap: '20px',
  },
  documentsSection: {
    marginTop: '48px',
  },
  sectionHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: '20px',
  },
  docCount: {
    fontSize: '14px',
    color: '#718096',
    fontFamily: "'JetBrains Mono', monospace",
  },
  emptyState: {
    textAlign: 'center',
    padding: '60px 20px',
    background: 'rgba(26, 32, 53, 0.5)',
    borderRadius: '16px',
    border: '1px dashed rgba(74, 85, 104, 0.4)',
  },
  emptyText: {
    color: '#718096',
    marginBottom: '16px',
  },
  emptyBtn: {
    padding: '12px 24px',
    fontSize: '14px',
    fontWeight: '500',
    borderRadius: '10px',
    border: '1px solid rgba(56, 178, 172, 0.3)',
    background: 'transparent',
    color: '#38b2ac',
    cursor: 'pointer',
  },
  dataSources: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    marginTop: '32px',
    padding: '16px',
    background: 'rgba(26, 32, 53, 0.5)',
    borderRadius: '12px',
  },
  dataSourcesLabel: {
    fontSize: '13px',
    color: '#718096',
  },
  dataSourceBadge: {
    fontSize: '11px',
    fontFamily: "'JetBrains Mono', monospace",
    padding: '4px 8px',
    background: 'rgba(56, 178, 172, 0.1)',
    border: '1px solid rgba(56, 178, 172, 0.2)',
    borderRadius: '6px',
    color: '#4fd1c5',
  },
  dataSourceMore: {
    fontSize: '12px',
    color: '#718096',
  },
  warnings: {
    marginTop: '24px',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  warning: {
    padding: '12px 16px',
    background: 'rgba(237, 137, 54, 0.1)',
    border: '1px solid rgba(237, 137, 54, 0.2)',
    borderRadius: '10px',
    color: '#ed8936',
    fontSize: '14px',
  },
  modal: {
    position: 'fixed',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    background: 'rgba(0, 0, 0, 0.7)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 1000,
    backdropFilter: 'blur(4px)',
  },
  modalContent: {
    position: 'relative',
    background: 'linear-gradient(145deg, rgba(26, 32, 53, 0.98) 0%, rgba(20, 26, 43, 0.98) 100%)',
    borderRadius: '20px',
    padding: '32px',
    maxWidth: '500px',
    width: '90%',
    border: '1px solid rgba(56, 178, 172, 0.15)',
    boxShadow: '0 25px 80px rgba(0, 0, 0, 0.5)',
  },
  closeBtn: {
    position: 'absolute',
    top: '16px',
    right: '16px',
    width: '32px',
    height: '32px',
    borderRadius: '8px',
    border: 'none',
    background: 'rgba(74, 85, 104, 0.3)',
    color: '#a0aec0',
    fontSize: '20px',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  settingsModalContent: {
    position: 'relative',
    background: 'linear-gradient(145deg, rgba(26, 32, 53, 0.98) 0%, rgba(20, 26, 43, 0.98) 100%)',
    borderRadius: '20px',
    padding: '32px',
    maxWidth: '480px',
    width: '90%',
    border: '1px solid rgba(56, 178, 172, 0.15)',
    boxShadow: '0 25px 80px rgba(0, 0, 0, 0.5)',
  },
  settingsTitle: {
    fontSize: '20px',
    fontWeight: '600',
    color: '#e2e8f0',
    marginBottom: '24px',
  },
  settingsSection: {
    marginBottom: '24px',
  },
  settingsSectionTitle: {
    fontSize: '14px',
    fontWeight: '500',
    color: '#718096',
    marginBottom: '16px',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  settingsItem: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '16px',
    background: 'rgba(26, 32, 53, 0.5)',
    borderRadius: '12px',
    marginBottom: '12px',
    border: '1px solid rgba(74, 85, 104, 0.2)',
  },
  settingsItemInfo: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
    flex: 1,
  },
  settingsItemTitle: {
    fontSize: '14px',
    fontWeight: '500',
    color: '#e2e8f0',
  },
  settingsItemDesc: {
    fontSize: '12px',
    color: '#718096',
  },
  resetDocsBtn: {
    padding: '8px 16px',
    fontSize: '13px',
    fontWeight: '500',
    borderRadius: '8px',
    border: '1px solid rgba(237, 137, 54, 0.4)',
    background: 'rgba(237, 137, 54, 0.1)',
    color: '#ed8936',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
  },
  switchCompanyBtn: {
    padding: '8px 16px',
    fontSize: '13px',
    fontWeight: '500',
    borderRadius: '8px',
    border: '1px solid rgba(245, 101, 101, 0.4)',
    background: 'rgba(245, 101, 101, 0.1)',
    color: '#f56565',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
  },
  settingsInfo: {
    padding: '12px 16px',
    background: 'rgba(56, 178, 172, 0.05)',
    borderRadius: '10px',
    border: '1px solid rgba(56, 178, 172, 0.1)',
    fontSize: '13px',
    color: '#718096',
  },
};

export default Dashboard;

