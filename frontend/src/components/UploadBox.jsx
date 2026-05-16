import React, { useState, useRef } from 'react';
import { uploadDocument } from '../api.js';

function UploadBox({ onComplete }) {
  const [file, setFile] = useState(null);
  const [docType, setDocType] = useState('P&L');
  const [periodStart, setPeriodStart] = useState('');
  const [periodEnd, setPeriodEnd] = useState('');
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState('');
  const [dragActive, setDragActive] = useState(false);
  const inputRef = useRef(null);

  function handleDrag(e) {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === 'dragenter' || e.type === 'dragover') {
      setDragActive(true);
    } else if (e.type === 'dragleave') {
      setDragActive(false);
    }
  }

  function handleDrop(e) {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);

    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      handleFile(e.dataTransfer.files[0]);
    }
  }

  function handleChange(e) {
    if (e.target.files && e.target.files[0]) {
      handleFile(e.target.files[0]);
    }
  }

  function handleFile(file) {
    const validTypes = ['.pdf', '.csv', '.xlsx'];
    const ext = file.name.substring(file.name.lastIndexOf('.')).toLowerCase();
    
    if (!validTypes.includes(ext)) {
      setError('Invalid file type. Please upload PDF, CSV, or XLSX files.');
      return;
    }

    setFile(file);
    setError('');
  }

  async function handleUpload() {
    if (!file) return;

    setUploading(true);
    setError('');

    try {
      await uploadDocument(file, docType, periodStart, periodEnd);
      onComplete();
    } catch (err) {
      setError(err.message || 'Failed to upload file');
    } finally {
      setUploading(false);
    }
  }

  function formatFileSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  }

  return (
    <div style={styles.container}>
      <h2 style={styles.title}>Upload Financial Document</h2>
      <p style={styles.subtitle}>
        Upload your P&L, Balance Sheet, or Cash Flow statements
      </p>

      {/* Drop Zone */}
      <div
        style={{
          ...styles.dropZone,
          ...(dragActive ? styles.dropZoneActive : {}),
          ...(file ? styles.dropZoneWithFile : {}),
        }}
        onDragEnter={handleDrag}
        onDragLeave={handleDrag}
        onDragOver={handleDrag}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
      >
        <input
          ref={inputRef}
          type="file"
          accept=".pdf,.csv,.xlsx"
          onChange={handleChange}
          style={styles.hiddenInput}
        />
        
        {file ? (
          <div style={styles.filePreview}>
            <span style={styles.fileIcon}>📄</span>
            <div>
              <p style={styles.fileName}>{file.name}</p>
              <p style={styles.fileSize}>{formatFileSize(file.size)}</p>
            </div>
            <button
              style={styles.removeBtn}
              onClick={(e) => {
                e.stopPropagation();
                setFile(null);
              }}
            >
              ×
            </button>
          </div>
        ) : (
          <>
            <div style={styles.dropIcon}>↑</div>
            <p style={styles.dropText}>
              Drop your file here or <span style={styles.browseLink}>browse</span>
            </p>
            <p style={styles.dropHint}>
              Supports PDF, CSV, XLSX
            </p>
          </>
        )}
      </div>

      {/* Form Fields */}
      <div style={styles.form}>
        <div style={styles.field}>
          <label style={styles.label}>Document Type</label>
          <select
            value={docType}
            onChange={(e) => setDocType(e.target.value)}
            style={styles.select}
          >
            <option value="P&L">Profit & Loss (P&L)</option>
            <option value="BalanceSheet">Balance Sheet</option>
            <option value="CashFlow">Cash Flow Statement</option>
            <option value="Unknown">Other</option>
          </select>
        </div>

        <div style={styles.row}>
          <div style={styles.field}>
            <label style={styles.label}>Period Start</label>
            <input
              type="date"
              value={periodStart}
              onChange={(e) => setPeriodStart(e.target.value)}
              style={styles.input}
            />
          </div>
          <div style={styles.field}>
            <label style={styles.label}>Period End</label>
            <input
              type="date"
              value={periodEnd}
              onChange={(e) => setPeriodEnd(e.target.value)}
              style={styles.input}
            />
          </div>
        </div>
      </div>

      {/* Error */}
      {error && (
        <div style={styles.error}>
          {error}
        </div>
      )}

      {/* Upload Button */}
      <button
        style={styles.uploadBtn}
        onClick={handleUpload}
        disabled={!file || uploading}
      >
        {uploading ? 'Uploading...' : 'Upload Document'}
      </button>
    </div>
  );
}

const styles = {
  container: {
    padding: '8px',
  },
  title: {
    fontSize: '20px',
    fontWeight: '600',
    color: '#f7fafc',
    marginBottom: '8px',
  },
  subtitle: {
    fontSize: '14px',
    color: '#a0aec0',
    marginBottom: '24px',
  },
  dropZone: {
    border: '2px dashed rgba(74, 85, 104, 0.4)',
    borderRadius: '16px',
    padding: '40px 20px',
    textAlign: 'center',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
    background: 'rgba(13, 17, 28, 0.3)',
  },
  dropZoneActive: {
    borderColor: '#38b2ac',
    background: 'rgba(56, 178, 172, 0.05)',
  },
  dropZoneWithFile: {
    padding: '20px',
    borderStyle: 'solid',
    borderColor: 'rgba(56, 178, 172, 0.3)',
  },
  hiddenInput: {
    display: 'none',
  },
  dropIcon: {
    fontSize: '32px',
    color: '#38b2ac',
    marginBottom: '12px',
  },
  dropText: {
    fontSize: '15px',
    color: '#a0aec0',
    marginBottom: '8px',
  },
  browseLink: {
    color: '#38b2ac',
    textDecoration: 'underline',
  },
  dropHint: {
    fontSize: '12px',
    color: '#718096',
  },
  filePreview: {
    display: 'flex',
    alignItems: 'center',
    gap: '16px',
    textAlign: 'left',
  },
  fileIcon: {
    fontSize: '32px',
  },
  fileName: {
    color: '#f7fafc',
    fontSize: '15px',
    fontWeight: '500',
  },
  fileSize: {
    color: '#718096',
    fontSize: '13px',
    fontFamily: "'JetBrains Mono', monospace",
  },
  removeBtn: {
    marginLeft: 'auto',
    width: '28px',
    height: '28px',
    borderRadius: '8px',
    border: 'none',
    background: 'rgba(245, 101, 101, 0.2)',
    color: '#fc8181',
    fontSize: '18px',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  form: {
    marginTop: '24px',
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
  },
  field: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    flex: 1,
  },
  row: {
    display: 'flex',
    gap: '12px',
  },
  label: {
    fontSize: '12px',
    fontWeight: '500',
    color: '#a0aec0',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  select: {
    padding: '12px 14px',
    fontSize: '14px',
    borderRadius: '10px',
    border: '1px solid rgba(74, 85, 104, 0.4)',
    background: 'rgba(13, 17, 28, 0.6)',
    color: '#f7fafc',
    outline: 'none',
    cursor: 'pointer',
    appearance: 'none',
    backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' fill='none' viewBox='0 0 24 24' stroke='%23a0aec0'%3E%3Cpath stroke-linecap='round' stroke-linejoin='round' stroke-width='2' d='M19 9l-7 7-7-7'/%3E%3C/svg%3E")`,
    backgroundRepeat: 'no-repeat',
    backgroundPosition: 'right 10px center',
    backgroundSize: '18px',
    paddingRight: '36px',
  },
  input: {
    padding: '12px 14px',
    fontSize: '14px',
    borderRadius: '10px',
    border: '1px solid rgba(74, 85, 104, 0.4)',
    background: 'rgba(13, 17, 28, 0.6)',
    color: '#f7fafc',
    outline: 'none',
  },
  error: {
    marginTop: '16px',
    padding: '12px 16px',
    background: 'rgba(245, 101, 101, 0.15)',
    border: '1px solid rgba(245, 101, 101, 0.3)',
    borderRadius: '10px',
    color: '#fc8181',
    fontSize: '14px',
  },
  uploadBtn: {
    marginTop: '24px',
    width: '100%',
    padding: '14px 24px',
    fontSize: '15px',
    fontWeight: '600',
    borderRadius: '12px',
    border: 'none',
    background: 'linear-gradient(135deg, #38b2ac 0%, #319795 100%)',
    color: '#fff',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
  },
};

export default UploadBox;

