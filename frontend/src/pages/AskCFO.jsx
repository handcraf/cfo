import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { askCFO } from '../api.js';

function AskCFO() {
  const [question, setQuestion] = useState('');
  const [response, setResponse] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  async function handleSubmit(e) {
    e.preventDefault();
    if (!question.trim()) return;

    setLoading(true);
    setError('');
    setResponse(null);

    try {
      const result = await askCFO(question);
      setResponse(result);
    } catch (err) {
      setError(err.message || 'Failed to get response');
    } finally {
      setLoading(false);
    }
  }

  const suggestedQuestions = [
    "What's our current cash position?",
    "How long is our runway?",
    "Explain our burn rate",
    "What are the revenue trends?",
    "Should I be worried about our finances?",
  ];

  return (
    <div style={styles.container}>
      {/* Header */}
      <header style={styles.header}>
        <Link to="/dashboard" style={styles.backLink}>
          ← Dashboard
        </Link>
        <div style={styles.logo}>
          <span style={styles.logoIcon}>◈</span>
          <span style={styles.logoText}>Ask AI CFO</span>
        </div>
        <div style={styles.spacer}></div>
      </header>

      {/* Main Content */}
      <main style={styles.main}>
        <div style={styles.chatContainer}>
          {/* Question Input */}
          <form onSubmit={handleSubmit} style={styles.form}>
            <div style={styles.inputWrapper}>
              <textarea
                value={question}
                onChange={(e) => setQuestion(e.target.value)}
                placeholder="Ask me anything about your finances..."
                style={styles.input}
                rows={3}
                disabled={loading}
              />
              <button
                type="submit"
                style={styles.submitBtn}
                disabled={loading || !question.trim()}
              >
                {loading ? (
                  <span style={styles.loadingDots}>●●●</span>
                ) : (
                  '→'
                )}
              </button>
            </div>
          </form>

          {/* Suggested Questions */}
          {!response && !loading && (
            <div style={styles.suggestions}>
              <p style={styles.suggestionsLabel}>Try asking:</p>
              <div style={styles.suggestionsList}>
                {suggestedQuestions.map((q, i) => (
                  <button
                    key={i}
                    style={styles.suggestionBtn}
                    onClick={() => setQuestion(q)}
                  >
                    {q}
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Loading State */}
          {loading && (
            <div style={styles.loadingCard}>
              <div style={styles.loadingPulse}></div>
              <p style={styles.loadingText}>
                AI CFO is analyzing your financial data...
              </p>
            </div>
          )}

          {/* Error State */}
          {error && (
            <div style={styles.errorCard}>
              <div style={styles.errorIcon}>⚠️</div>
              <div>
                <p style={styles.errorTitle}>Unable to process question</p>
                <p style={styles.errorText}>{error}</p>
              </div>
            </div>
          )}

          {/* Response */}
          {response && !loading && (
            <div style={styles.responseCard}>
              {/* Question Echo */}
              <div style={styles.questionEcho}>
                <span style={styles.youLabel}>You asked:</span>
                <p style={styles.questionText}>{response.question}</p>
              </div>

              {/* Summary */}
              <div style={styles.summarySection}>
                <div style={styles.sectionIcon}>💡</div>
                <div>
                  <h3 style={styles.sectionTitle}>Summary</h3>
                  <p style={styles.summaryText}>{response.summary}</p>
                </div>
              </div>

              {/* Numbers Used */}
              {response.numbers_used?.length > 0 && (
                <div style={styles.numbersSection}>
                  <h3 style={styles.sectionTitle}>
                    <span style={styles.sectionIcon}>📊</span>
                    Numbers Used
                  </h3>
                  <div style={styles.numbersList}>
                    {response.numbers_used.map((num, i) => (
                      <div key={i} style={styles.numberItem}>
                        {num}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Explanation */}
              {response.explanation && (
                <div style={styles.explanationSection}>
                  <h3 style={styles.sectionTitle}>
                    <span style={styles.sectionIcon}>📝</span>
                    Detailed Explanation
                  </h3>
                  <p style={styles.explanationText}>{response.explanation}</p>
                </div>
              )}

              {/* Sources */}
              {response.sources?.length > 0 && (
                <div style={styles.sourcesSection}>
                  <span style={styles.sourcesLabel}>Sources:</span>
                  {response.sources.map((src, i) => (
                    <span key={i} style={styles.sourceBadge}>
                      {src.substring(0, 12)}...
                    </span>
                  ))}
                </div>
              )}

              {/* Ask Another */}
              <button
                style={styles.askAnotherBtn}
                onClick={() => {
                  setQuestion('');
                  setResponse(null);
                }}
              >
                Ask another question
              </button>
            </div>
          )}
        </div>
      </main>
    </div>
  );
}

const styles = {
  container: {
    minHeight: '100vh',
  },
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '20px 40px',
    borderBottom: '1px solid rgba(74, 85, 104, 0.2)',
  },
  backLink: {
    color: '#a0aec0',
    textDecoration: 'none',
    fontSize: '14px',
    padding: '8px 16px',
    borderRadius: '8px',
    transition: 'all 0.2s ease',
    flex: 1,
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
  spacer: {
    flex: 1,
  },
  main: {
    maxWidth: '800px',
    margin: '0 auto',
    padding: '40px 20px',
  },
  chatContainer: {
    display: 'flex',
    flexDirection: 'column',
    gap: '24px',
  },
  form: {},
  inputWrapper: {
    position: 'relative',
    background: 'linear-gradient(145deg, rgba(26, 32, 53, 0.9) 0%, rgba(20, 26, 43, 0.95) 100%)',
    borderRadius: '20px',
    border: '1px solid rgba(56, 178, 172, 0.2)',
    overflow: 'hidden',
  },
  input: {
    width: '100%',
    padding: '20px 60px 20px 24px',
    fontSize: '16px',
    border: 'none',
    background: 'transparent',
    color: '#f7fafc',
    resize: 'none',
    outline: 'none',
    fontFamily: 'inherit',
    lineHeight: '1.5',
  },
  submitBtn: {
    position: 'absolute',
    right: '12px',
    bottom: '12px',
    width: '44px',
    height: '44px',
    borderRadius: '12px',
    border: 'none',
    background: 'linear-gradient(135deg, #38b2ac 0%, #319795 100%)',
    color: '#fff',
    fontSize: '20px',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    transition: 'all 0.2s ease',
  },
  loadingDots: {
    fontSize: '14px',
    letterSpacing: '-2px',
    animation: 'pulse 1s infinite',
  },
  suggestions: {
    padding: '24px',
    background: 'rgba(26, 32, 53, 0.5)',
    borderRadius: '16px',
  },
  suggestionsLabel: {
    fontSize: '13px',
    color: '#718096',
    marginBottom: '16px',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  suggestionsList: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '10px',
  },
  suggestionBtn: {
    padding: '10px 16px',
    fontSize: '13px',
    borderRadius: '10px',
    border: '1px solid rgba(74, 85, 104, 0.3)',
    background: 'rgba(13, 17, 28, 0.5)',
    color: '#a0aec0',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
  },
  loadingCard: {
    display: 'flex',
    alignItems: 'center',
    gap: '16px',
    padding: '24px',
    background: 'rgba(26, 32, 53, 0.7)',
    borderRadius: '16px',
    border: '1px solid rgba(56, 178, 172, 0.1)',
  },
  loadingPulse: {
    width: '48px',
    height: '48px',
    borderRadius: '12px',
    background: 'linear-gradient(135deg, rgba(56, 178, 172, 0.3) 0%, rgba(56, 178, 172, 0.1) 100%)',
    animation: 'pulse 1.5s infinite',
  },
  loadingText: {
    color: '#a0aec0',
    fontSize: '15px',
  },
  errorCard: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '16px',
    padding: '20px',
    background: 'rgba(245, 101, 101, 0.1)',
    border: '1px solid rgba(245, 101, 101, 0.2)',
    borderRadius: '16px',
  },
  errorIcon: {
    fontSize: '24px',
  },
  errorTitle: {
    color: '#fc8181',
    fontWeight: '500',
    marginBottom: '4px',
  },
  errorText: {
    color: '#feb2b2',
    fontSize: '14px',
  },
  responseCard: {
    background: 'linear-gradient(145deg, rgba(26, 32, 53, 0.9) 0%, rgba(20, 26, 43, 0.95) 100%)',
    borderRadius: '20px',
    border: '1px solid rgba(56, 178, 172, 0.15)',
    padding: '28px',
    display: 'flex',
    flexDirection: 'column',
    gap: '24px',
  },
  questionEcho: {
    paddingBottom: '20px',
    borderBottom: '1px solid rgba(74, 85, 104, 0.2)',
  },
  youLabel: {
    fontSize: '12px',
    color: '#718096',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  questionText: {
    color: '#e2e8f0',
    fontSize: '16px',
    marginTop: '6px',
  },
  summarySection: {
    display: 'flex',
    gap: '16px',
    padding: '20px',
    background: 'rgba(56, 178, 172, 0.08)',
    borderRadius: '14px',
    border: '1px solid rgba(56, 178, 172, 0.15)',
  },
  sectionIcon: {
    fontSize: '20px',
    marginRight: '8px',
  },
  sectionTitle: {
    fontSize: '13px',
    color: '#4fd1c5',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
    marginBottom: '8px',
    display: 'flex',
    alignItems: 'center',
  },
  summaryText: {
    color: '#f7fafc',
    fontSize: '16px',
    lineHeight: '1.6',
  },
  numbersSection: {
    padding: '20px',
    background: 'rgba(13, 17, 28, 0.5)',
    borderRadius: '14px',
  },
  numbersList: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
    gap: '12px',
    marginTop: '12px',
  },
  numberItem: {
    padding: '12px 16px',
    background: 'rgba(26, 32, 53, 0.8)',
    borderRadius: '10px',
    border: '1px solid rgba(74, 85, 104, 0.2)',
    color: '#e2e8f0',
    fontSize: '14px',
    fontFamily: "'JetBrains Mono', monospace",
  },
  explanationSection: {
    padding: '20px',
    background: 'rgba(13, 17, 28, 0.5)',
    borderRadius: '14px',
  },
  explanationText: {
    color: '#cbd5e0',
    fontSize: '15px',
    lineHeight: '1.7',
    marginTop: '12px',
  },
  sourcesSection: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    flexWrap: 'wrap',
    paddingTop: '16px',
    borderTop: '1px solid rgba(74, 85, 104, 0.2)',
  },
  sourcesLabel: {
    fontSize: '12px',
    color: '#718096',
  },
  sourceBadge: {
    fontSize: '11px',
    fontFamily: "'JetBrains Mono', monospace",
    padding: '4px 8px',
    background: 'rgba(74, 85, 104, 0.2)',
    borderRadius: '6px',
    color: '#a0aec0',
  },
  askAnotherBtn: {
    alignSelf: 'center',
    padding: '12px 24px',
    fontSize: '14px',
    fontWeight: '500',
    borderRadius: '10px',
    border: '1px solid rgba(56, 178, 172, 0.3)',
    background: 'transparent',
    color: '#38b2ac',
    cursor: 'pointer',
    marginTop: '8px',
    transition: 'all 0.2s ease',
  },
};

// Add keyframes
const styleSheet = document.createElement('style');
styleSheet.textContent = `
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }
`;
document.head.appendChild(styleSheet);

export default AskCFO;

