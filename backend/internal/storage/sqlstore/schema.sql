-- AI CFO SQLite schema.
--
-- Design principles:
--   - YYYY-MM-DD for all dates so lexical compare == temporal compare.
--   - Foreign keys ON (enabled in DSN). Cascades defined inline.
--   - BLOBs and JSON columns intentionally avoided for forensic clarity.
--   - Single writer: no concurrency-hostile constructs.
--
-- Every ALTER must be tracked in a migration (see sqlstore.go TODO).

CREATE TABLE IF NOT EXISTS companies (
    id              INTEGER PRIMARY KEY,       -- fixed to 1 today; tenant_id later
    name            TEXT NOT NULL,
    industry        TEXT,
    industry_type   TEXT,                      -- generic|education|ecommerce|pharma
    fiscal_year_end TEXT,
    currency        TEXT,
    setup_completed INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS documents (
    id           TEXT PRIMARY KEY,              -- slug/safe id from the filename
    filename     TEXT NOT NULL,
    doc_type     TEXT NOT NULL,                 -- P&L|BalanceSheet|CashFlow|Unknown
    period_start TEXT NOT NULL,                 -- YYYY-MM-DD
    period_end   TEXT NOT NULL,                 -- YYYY-MM-DD
    file_path    TEXT NOT NULL,                 -- raw uploaded file location
    parsed_path  TEXT NOT NULL,                 -- parsed JSON location
    file_size    INTEGER NOT NULL,
    mime_type    TEXT,
    uploaded_at  DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_documents_period
    ON documents(period_start, period_end);

CREATE INDEX IF NOT EXISTS idx_documents_doctype
    ON documents(doc_type);

-- line_items is the per-document key/value store of parsed financials.
-- Example rows:
--   (doc_q1_2024, "revenue", 1250000)
--   (doc_q1_2024, "cash", 890000)
-- The metric_key vocabulary is controlled by the parser (see
-- internal/service/parsing/* — not in this PR).
CREATE TABLE IF NOT EXISTS line_items (
    document_id  TEXT NOT NULL,
    metric_key   TEXT NOT NULL,
    metric_value REAL NOT NULL,
    PRIMARY KEY (document_id, metric_key),
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_line_items_metric
    ON line_items(metric_key);

-- ask_audit is append-only. NEVER add UPDATE or DELETE paths to this table.
-- It is the forensic log for every question asked, used for:
--   - compliance (what did the CFO say when?)
--   - debugging (why was confidence Low?)
--   - learn-to-rank (future)
CREATE TABLE IF NOT EXISTS ask_audit (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    at            DATETIME NOT NULL,
    question      TEXT NOT NULL,
    period        TEXT,
    numbers_used  TEXT,        -- newline-separated for readability
    evidence_ids  TEXT,        -- comma-separated chunk IDs
    confidence    TEXT,        -- high|medium|low|unknown
    conflicts     INTEGER NOT NULL DEFAULT 0,
    error_msg     TEXT
);

CREATE INDEX IF NOT EXISTS idx_ask_audit_at
    ON ask_audit(at DESC);
