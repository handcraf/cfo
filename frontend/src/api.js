// API client for AI CFO backend
// In Docker: frontend proxies /api/* to backend via nginx
// For local dev: use 0.0.0.0:8080 directly

const getBackendUrl = () => {
  // Check if we're in Docker (proxied through nginx)
  if (window.location.port === '3000' || window.location.port === '80' || window.location.port === '') {
    // Use relative URLs which nginx will proxy
    return '/api';
  }
  // Local development fallback
  return 'http://0.0.0.0:8080';
};

const BACKEND_URL = getBackendUrl();

// ApiError carries the structured error payload from the backend so
// the UI can route on `error` / `reason` / `action` (e.g. show the
// LicenseError page on a 503 with error="license_invalid", or the
// LoginPage on a 401 with error="not_authenticated").
export class ApiError extends Error {
  constructor(status, body) {
    super(body?.message || body?.error || `HTTP ${status}`);
    this.status = status;
    this.body = body || {};
  }
}

// Helper for API calls. credentials: 'include' is required so the
// HTTP-only session cookie is sent on every request (via Vite proxy
// in dev, or nginx in prod).
async function apiCall(endpoint, options = {}) {
  const url = `${BACKEND_URL}${endpoint}`;

  const response = await fetch(url, {
    credentials: 'include',
    ...options,
    headers: {
      ...options.headers,
    },
  });

  if (!response.ok) {
    const body = await response.json().catch(() => ({ error: 'Request failed' }));
    throw new ApiError(response.status, body);
  }

  return response.json();
}

// ----------------------------------------------------------------------
// License + auth endpoints
// ----------------------------------------------------------------------

export async function getLicenseStatus() {
  return apiCall('/license/status');
}

export async function getAuthStatus() {
  return apiCall('/auth/status');
}

export async function authSetup(password) {
  return apiCall('/auth/setup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  });
}

export async function authLogin(password) {
  return apiCall('/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  });
}

export async function authLogout() {
  return apiCall('/auth/logout', { method: 'POST' });
}

// Health check
export async function checkHealth() {
  return apiCall('/health');
}

// Company setup
export async function setupCompany(companyData) {
  return apiCall('/setup/company', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(companyData),
  });
}

// Get company status
export async function getCompanyStatus() {
  return apiCall('/company/status');
}

// Upload document
export async function uploadDocument(file, docType, periodStart, periodEnd) {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('doc_type', docType);
  formData.append('period_start', periodStart);
  formData.append('period_end', periodEnd);
  
  return apiCall('/documents/upload', {
    method: 'POST',
    body: formData,
  });
}

// Get all documents
export async function getDocuments() {
  return apiCall('/documents');
}

// Get current metrics
export async function getMetrics() {
  return apiCall('/metrics/current');
}

// Ask CFO a question
export async function askCFO(question) {
  return apiCall('/ask', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ question }),
  });
}

// Reset company data (switch to new company)
export async function resetCompany() {
  return apiCall('/company/reset', {
    method: 'DELETE',
  });
}

// Reset all documents (keep company)
export async function resetDocuments() {
  return apiCall('/documents/reset', {
    method: 'DELETE',
  });
}
