import React from 'react';

function MetricCard({ title, value, trend, icon, color, invertTrend = false }) {
  // Determine if trend is positive (good) or negative (bad)
  const trendValue = trend ? parseFloat(trend) : null;
  const isPositive = trendValue !== null && 
    (invertTrend ? trendValue < 0 : trendValue > 0);
  const isNegative = trendValue !== null && 
    (invertTrend ? trendValue > 0 : trendValue < 0);

  return (
    <div style={styles.card}>
      <div style={styles.header}>
        <span style={styles.icon}>{icon}</span>
        <span style={styles.title}>{title}</span>
      </div>
      
      <div style={styles.valueWrapper}>
        <span style={{ ...styles.value, color }}>{value}</span>
      </div>
      
      {trend && (
        <div style={{
          ...styles.trend,
          color: isPositive ? '#48bb78' : isNegative ? '#f56565' : '#a0aec0',
          background: isPositive 
            ? 'rgba(72, 187, 120, 0.1)' 
            : isNegative 
              ? 'rgba(245, 101, 101, 0.1)' 
              : 'rgba(160, 174, 192, 0.1)',
        }}>
          <span style={styles.trendArrow}>
            {isPositive ? '↑' : isNegative ? '↓' : '→'}
          </span>
          {trend}
        </div>
      )}
      
      <div style={{ ...styles.glow, background: color }}></div>
    </div>
  );
}

const styles = {
  card: {
    position: 'relative',
    background: 'linear-gradient(145deg, rgba(26, 32, 53, 0.8) 0%, rgba(20, 26, 43, 0.9) 100%)',
    borderRadius: '16px',
    padding: '24px',
    border: '1px solid rgba(74, 85, 104, 0.2)',
    overflow: 'hidden',
    transition: 'all 0.3s ease',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
    marginBottom: '16px',
  },
  icon: {
    fontSize: '18px',
  },
  title: {
    fontSize: '13px',
    color: '#a0aec0',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
    fontWeight: '500',
  },
  valueWrapper: {
    marginBottom: '8px',
  },
  value: {
    fontSize: '28px',
    fontWeight: '600',
    fontFamily: "'JetBrains Mono', monospace",
    letterSpacing: '-1px',
  },
  trend: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '4px',
    padding: '4px 10px',
    borderRadius: '6px',
    fontSize: '12px',
    fontWeight: '500',
    fontFamily: "'JetBrains Mono', monospace",
  },
  trendArrow: {
    fontSize: '10px',
  },
  glow: {
    position: 'absolute',
    top: 0,
    right: 0,
    width: '100px',
    height: '100px',
    borderRadius: '50%',
    filter: 'blur(60px)',
    opacity: 0.15,
    pointerEvents: 'none',
  },
};

export default MetricCard;

