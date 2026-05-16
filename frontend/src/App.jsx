import React, { useState, useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate, useNavigate } from 'react-router-dom';
import { getCompanyStatus, getLicenseStatus, getAuthStatus } from './api.js';
import Setup from './pages/Setup.jsx';
import Dashboard from './pages/Dashboard.jsx';
import AskCFO from './pages/AskCFO.jsx';
import LoginPage from './pages/LoginPage.jsx';
import LicenseError from './pages/LicenseError.jsx';

// Global styles
const globalStyles = `
  * {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
  }
  
  body {
    font-family: 'Outfit', -apple-system, BlinkMacSystemFont, sans-serif;
    background: linear-gradient(135deg, #0a0e17 0%, #151c2c 50%, #1a1f35 100%);
    min-height: 100vh;
    color: #e4e8ef;
  }
  
  code, .mono {
    font-family: 'JetBrains Mono', monospace;
  }
`;

// Root component that handles routing based on setup status
function AppRouter() {
  const [isSetupDone, setIsSetupDone] = useState(null);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    checkSetup();
  }, []);

  async function checkSetup() {
    try {
      const status = await getCompanyStatus();
      setIsSetupDone(status.setup_completed);
      
      if (!status.setup_completed) {
        navigate('/setup');
      }
    } catch (error) {
      console.error('Failed to check setup status:', error);
      // If backend is not available, show setup anyway
      setIsSetupDone(false);
    } finally {
      setLoading(false);
    }
  }

  function handleSetupComplete() {
    setIsSetupDone(true);
    navigate('/dashboard');
  }

  if (loading) {
    return (
      <div style={styles.loading}>
        <div style={styles.loadingSpinner}></div>
        <p>Connecting to AI CFO...</p>
      </div>
    );
  }

  return (
    <Routes>
      <Route 
        path="/setup" 
        element={
          isSetupDone 
            ? <Navigate to="/dashboard" replace /> 
            : <Setup onComplete={handleSetupComplete} />
        } 
      />
      <Route 
        path="/dashboard" 
        element={
          isSetupDone 
            ? <Dashboard /> 
            : <Navigate to="/setup" replace />
        } 
      />
      <Route 
        path="/ask" 
        element={
          isSetupDone 
            ? <AskCFO /> 
            : <Navigate to="/setup" replace />
        } 
      />
      <Route 
        path="/" 
        element={<Navigate to={isSetupDone ? "/dashboard" : "/setup"} replace />} 
      />
    </Routes>
  );
}

// Gate is the outermost router-less wrapper. It checks license status
// FIRST (because a bad license blocks the whole product), then auth
// status. Only when both are green do we hand off to AppRouter.
function Gate() {
  const [phase, setPhase] = useState('loading'); // loading | license_bad | needs_login | ready
  const [license, setLicense] = useState(null);
  const [needsSetup, setNeedsSetup] = useState(false);

  async function reload() {
    setPhase('loading');
    try {
      const lic = await getLicenseStatus();
      setLicense(lic);
      if (!lic.ok) {
        setPhase('license_bad');
        return;
      }
      const auth = await getAuthStatus();
      setNeedsSetup(!!auth.needs_setup);
      if (auth.authenticated) {
        setPhase('ready');
      } else {
        setPhase('needs_login');
      }
    } catch (err) {
      // Likely network error (backend down). Treat as license-bad
      // with a synthetic reason so the user sees a helpful screen
      // instead of an infinite spinner.
      setLicense({
        ok: false,
        reason: 'unreachable',
        message: 'Backend is not reachable. Is it running on port 8080?',
      });
      setPhase('license_bad');
    }
  }

  useEffect(() => { reload(); }, []);

  if (phase === 'loading') {
    return (
      <div style={styles.loading}>
        <div style={styles.loadingSpinner}></div>
        <p>Connecting to AI CFO...</p>
      </div>
    );
  }
  if (phase === 'license_bad') return <LicenseError status={license} onRetry={reload} />;
  if (phase === 'needs_login') return <LoginPage needsSetup={needsSetup} onSuccess={reload} />;

  return (
    <BrowserRouter>
      <AppRouter />
    </BrowserRouter>
  );
}

function App() {
  return (
    <>
      <style>{globalStyles}</style>
      <Gate />
    </>
  );
}

const styles = {
  loading: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    height: '100vh',
    gap: '20px',
    color: '#a0aec0',
  },
  loadingSpinner: {
    width: '40px',
    height: '40px',
    border: '3px solid #2d3748',
    borderTopColor: '#38b2ac',
    borderRadius: '50%',
    animation: 'spin 1s linear infinite',
  },
};

// Add keyframes for spinner
const styleSheet = document.createElement('style');
styleSheet.textContent = `
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
`;
document.head.appendChild(styleSheet);

export default App;

