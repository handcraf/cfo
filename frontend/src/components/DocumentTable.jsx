import React from 'react';

function DocumentTable({ documents }) {
  if (!documents || documents.length === 0) {
    return null;
  }

  function formatDate(dateString) {
    if (!dateString) return '—';
    const date = new Date(dateString);
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  }

  function formatFileSize(bytes) {
    if (!bytes) return '—';
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  }

  function getDocTypeColor(docType) {
    switch (docType) {
      case 'P&L':
        return { bg: 'rgba(72, 187, 120, 0.15)', color: '#48bb78' };
      case 'BalanceSheet':
        return { bg: 'rgba(66, 153, 225, 0.15)', color: '#4299e1' };
      case 'CashFlow':
        return { bg: 'rgba(159, 122, 234, 0.15)', color: '#9f7aea' };
      default:
        return { bg: 'rgba(160, 174, 192, 0.15)', color: '#a0aec0' };
    }
  }

  function getFileIcon(filename) {
    const ext = filename.split('.').pop().toLowerCase();
    switch (ext) {
      case 'pdf':
        return '📄';
      case 'csv':
        return '📊';
      case 'xlsx':
        return '📈';
      default:
        return '📁';
    }
  }

  return (
    <div style={styles.container}>
      <table style={styles.table}>
        <thead>
          <tr>
            <th style={styles.th}>File</th>
            <th style={styles.th}>Type</th>
            <th style={styles.th}>Period</th>
            <th style={styles.th}>Size</th>
            <th style={styles.th}>Uploaded</th>
          </tr>
        </thead>
        <tbody>
          {documents.map((doc) => {
            const typeColors = getDocTypeColor(doc.doc_type);
            return (
              <tr key={doc.id} style={styles.tr}>
                <td style={styles.td}>
                  <div style={styles.fileCell}>
                    <span style={styles.fileIcon}>
                      {getFileIcon(doc.filename)}
                    </span>
                    <div>
                      <div style={styles.filename}>{doc.filename}</div>
                      <div style={styles.fileId}>{doc.id}</div>
                    </div>
                  </div>
                </td>
                <td style={styles.td}>
                  <span style={{
                    ...styles.typeBadge,
                    background: typeColors.bg,
                    color: typeColors.color,
                  }}>
                    {doc.doc_type}
                  </span>
                </td>
                <td style={styles.td}>
                  <div style={styles.period}>
                    {doc.period_start && doc.period_end ? (
                      <>
                        <span>{formatDate(doc.period_start)}</span>
                        <span style={styles.periodSep}>→</span>
                        <span>{formatDate(doc.period_end)}</span>
                      </>
                    ) : (
                      <span style={styles.periodNA}>Not specified</span>
                    )}
                  </div>
                </td>
                <td style={styles.td}>
                  <span style={styles.fileSize}>
                    {formatFileSize(doc.file_size)}
                  </span>
                </td>
                <td style={styles.td}>
                  <span style={styles.date}>
                    {formatDate(doc.uploaded_at)}
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

const styles = {
  container: {
    background: 'linear-gradient(145deg, rgba(26, 32, 53, 0.6) 0%, rgba(20, 26, 43, 0.7) 100%)',
    borderRadius: '16px',
    border: '1px solid rgba(74, 85, 104, 0.2)',
    overflow: 'hidden',
  },
  table: {
    width: '100%',
    borderCollapse: 'collapse',
  },
  th: {
    textAlign: 'left',
    padding: '16px 20px',
    fontSize: '11px',
    fontWeight: '600',
    color: '#718096',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
    borderBottom: '1px solid rgba(74, 85, 104, 0.2)',
    background: 'rgba(13, 17, 28, 0.3)',
  },
  tr: {
    transition: 'background 0.2s ease',
  },
  td: {
    padding: '16px 20px',
    borderBottom: '1px solid rgba(74, 85, 104, 0.1)',
    verticalAlign: 'middle',
  },
  fileCell: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
  },
  fileIcon: {
    fontSize: '20px',
  },
  filename: {
    color: '#e2e8f0',
    fontSize: '14px',
    fontWeight: '500',
    marginBottom: '2px',
  },
  fileId: {
    color: '#718096',
    fontSize: '11px',
    fontFamily: "'JetBrains Mono', monospace",
  },
  typeBadge: {
    display: 'inline-block',
    padding: '4px 10px',
    borderRadius: '6px',
    fontSize: '12px',
    fontWeight: '500',
  },
  period: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '13px',
    color: '#a0aec0',
    fontFamily: "'JetBrains Mono', monospace",
  },
  periodSep: {
    color: '#4a5568',
    fontSize: '11px',
  },
  periodNA: {
    color: '#718096',
    fontStyle: 'italic',
  },
  fileSize: {
    fontSize: '13px',
    color: '#a0aec0',
    fontFamily: "'JetBrains Mono', monospace",
  },
  date: {
    fontSize: '13px',
    color: '#a0aec0',
  },
};

export default DocumentTable;

