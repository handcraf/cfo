# AI CFO - Complete Documentation

## 📋 Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Project Structure](#project-structure)
4. [Backend API Reference](#backend-api-reference)
5. [Frontend Components](#frontend-components)
6. [Data Models](#data-models)
7. [Services](#services)
8. [Configuration](#configuration)
9. [Deployment](#deployment)
10. [Development Guide](#development-guide)

---

## Overview

### What is AI CFO?

AI CFO is an **on-premises AI-powered financial advisor** that:
- Understands a company's financial history
- Tracks changes over time
- Explains business decisions using local LLM
- **Never sends data to the cloud**

### Key Features

| Feature | Description |
|---------|-------------|
| **Document Upload** | Upload P&L, Balance Sheet, Cash Flow statements (CSV, PDF, XLSX) |
| **Financial Metrics** | Automatic calculation of Cash, Burn, Runway, Revenue, Expenses |
| **Trend Analysis** | Period-over-period comparison with % changes |
| **AI Explanations** | Natural language explanations powered by local Ollama LLM |
| **No Database** | All data stored as JSON files |
| **No Cloud** | Everything runs locally, no internet required |

### Design Principles

1. **LLM explains, code calculates** - All financial metrics are calculated deterministically in Go code, not by LLM
2. **File-based storage** - No database dependencies, easy to backup and inspect
3. **Privacy first** - All data stays on your machine
4. **Hackathon-ready** - Minimal dependencies, easy to run

---

## Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLIENT                                   │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                   React Frontend                         │   │
│  │   Setup Page │ Dashboard │ Ask CFO                       │   │
│  └──────────────────────┬──────────────────────────────────┘   │
└─────────────────────────┼───────────────────────────────────────┘
                          │ HTTP/REST
┌─────────────────────────┼───────────────────────────────────────┐
│                      BACKEND                                     │
│  ┌──────────────────────┴──────────────────────────────────┐   │
│  │                    Go HTTP Server                        │   │
│  │   /health │ /setup │ /documents │ /metrics │ /ask        │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │                                        │
│  ┌──────────────────────┴──────────────────────────────────┐   │
│  │                    Services Layer                        │   │
│  │   DocumentParser │ FinancialLogic │ RAG │ LLM            │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │                                        │
│  ┌──────────────────────┴──────────────────────────────────┐   │
│  │                    Storage Layer                         │   │
│  │   FileStore (JSON files)                                 │   │
│  └──────────────────────┬──────────────────────────────────┘   │
└─────────────────────────┼───────────────────────────────────────┘
                          │
┌─────────────────────────┼───────────────────────────────────────┐
│                    FILE SYSTEM                                   │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐               │
│  │ /documents/ │ │  /parsed/   │ │   /state/   │               │
│  │ (raw files) │ │   (JSON)    │ │   (JSON)    │               │
│  └─────────────┘ └─────────────┘ └─────────────┘               │
└─────────────────────────────────────────────────────────────────┘
                          │
┌─────────────────────────┼───────────────────────────────────────┐
│                    OLLAMA LLM                                    │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │   Local LLM (llama3 / tinyllama)                         │   │
│  │   HTTP API on :11434                                     │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **User uploads document** → Saved to `/documents/` → Parsed to `/parsed/`
2. **User views dashboard** → Backend reads `/parsed/` → Calculates metrics → Returns JSON
3. **User asks question** → Backend calculates metrics → RAG finds context → LLM explains → Returns answer

---

## Project Structure

```
/cfo
├── backend/
│   ├── cmd/server/
│   │   └── main.go                 # Application entry point
│   ├── internal/
│   │   ├── api/                    # HTTP handlers
│   │   │   ├── ask.go              # POST /ask
│   │   │   ├── documents.go        # POST/GET /documents
│   │   │   ├── health.go           # GET /health
│   │   │   ├── metrics.go          # GET /metrics
│   │   │   ├── setup.go            # Routes + company setup
│   │   │   └── handlers_test.go    # API tests
│   │   ├── service/                # Business logic
│   │   │   ├── document_parser.go  # CSV/PDF/XLSX parsing
│   │   │   ├── financial_logic.go  # Metric calculations
│   │   │   ├── rag.go              # Retrieval augmented generation
│   │   │   ├── llm.go              # Ollama integration
│   │   │   └── *_test.go           # Service tests
│   │   ├── storage/                # File-based storage
│   │   │   ├── filesystem.go       # CRUD operations
│   │   │   ├── paths.go            # Path helpers
│   │   │   └── filesystem_test.go  # Storage tests
│   │   ├── model/                  # Data structures
│   │   │   ├── company.go
│   │   │   ├── document.go
│   │   │   └── metrics.go
│   │   └── config/
│   │       └── config.go           # Environment config
│   ├── data/                       # Data storage
│   │   ├── documents/              # Uploaded raw files
│   │   ├── parsed/                 # Parsed JSON
│   │   └── state/                  # App state (company.json, documents.json)
│   ├── testdata/                   # Test CSV files
│   ├── Dockerfile
│   ├── go.mod
│   ├── integration_test.go
│   └── e2e_test.go
│
├── frontend/
│   ├── src/
│   │   ├── pages/
│   │   │   ├── Setup.jsx           # Company setup form
│   │   │   ├── Dashboard.jsx       # Main dashboard
│   │   │   └── AskCFO.jsx          # Q&A interface
│   │   ├── components/
│   │   │   ├── MetricCard.jsx      # Metric display card
│   │   │   ├── DocumentTable.jsx   # Document list
│   │   │   └── UploadBox.jsx       # File upload
│   │   ├── api.js                  # API client
│   │   ├── App.jsx                 # Main app + routing
│   │   └── main.jsx                # Entry point
│   ├── package.json
│   ├── vite.config.js
│   ├── nginx.conf
│   └── Dockerfile
│
├── diagrams/                       # Architecture diagrams
├── docs/                           # Documentation
├── docker-compose.yml
├── run.md
├── TESTING.md
└── .gitignore
```

---

## Backend API Reference

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "service": "ai-cfo-backend"
}
```

---

### POST /setup/company

Setup company information.

**Request:**
```json
{
  "name": "Acme Corp",
  "industry": "Technology",
  "fiscal_year_end": "December",
  "currency": "USD"
}
```

**Response:**
```json
{
  "name": "Acme Corp",
  "industry": "Technology",
  "fiscal_year_end": "December",
  "currency": "USD",
  "setup_completed": true,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T12:00:00Z"
}
```

**Errors:**
- `400` - Company name is required
- `400` - Invalid request body

---

### GET /company/status

Get company setup status.

**Response:**
```json
{
  "setup_completed": true,
  "company": {
    "name": "Acme Corp",
    "industry": "Technology",
    ...
  }
}
```

---

### POST /documents/upload

Upload a financial document.

**Request:** `multipart/form-data`
- `file` - The file (CSV, PDF, XLSX)
- `doc_type` - "P&L", "BalanceSheet", "CashFlow", "Unknown"
- `period_start` - Start date (YYYY-MM-DD)
- `period_end` - End date (YYYY-MM-DD)

**Response:**
```json
{
  "id": "doc_1234567890",
  "filename": "q1_financials.csv",
  "doc_type": "P&L",
  "period_start": "2024-01-01",
  "period_end": "2024-03-31",
  "file_path": "/data/documents/q1_financials.csv",
  "parsed_path": "/data/parsed/doc_1234567890.json",
  "uploaded_at": "2024-04-01T12:00:00Z",
  "file_size": 1024,
  "mime_type": "text/csv"
}
```

**Errors:**
- `400` - No file provided
- `400` - Invalid file type

---

### GET /documents

List all uploaded documents.

**Response:**
```json
{
  "documents": [
    {
      "id": "doc_123",
      "filename": "q1.csv",
      "doc_type": "P&L",
      "period_start": "2024-01-01",
      "period_end": "2024-03-31",
      "uploaded_at": "2024-04-01T12:00:00Z"
    }
  ],
  "updated_at": "2024-04-01T12:00:00Z"
}
```

---

### GET /metrics/current

Get calculated financial metrics.

**Response:**
```json
{
  "cash": 500000,
  "monthly_burn": 50000,
  "runway_months": 10,
  "revenue": 1000000,
  "expenses": 800000,
  "net_income": 200000,
  "total_assets": 2000000,
  "total_liabilities": 800000,
  "equity": 1200000,
  "period_start": "2024-01-01",
  "period_end": "2024-03-31",
  "trends": {
    "revenue_change_pct": 20.0,
    "expense_change_pct": 5.0,
    "cash_change_pct": 15.0
  },
  "data_sources": ["doc_123", "doc_456"],
  "errors": []
}
```

---

### POST /ask

Ask the AI CFO a question.

**Request:**
```json
{
  "question": "What is our current cash position?"
}
```

**Response:**
```json
{
  "question": "What is our current cash position?",
  "summary": "Your cash position is $500,000 with 10 months of runway.",
  "numbers_used": [
    "Cash: $500,000.00",
    "Monthly Burn: $50,000.00",
    "Runway: 10 months"
  ],
  "explanation": "Based on your current cash reserves and monthly burn rate...",
  "sources": ["doc_123", "doc_456"],
  "error": ""
}
```

---

## Frontend Components

### Pages

| Page | Route | Description |
|------|-------|-------------|
| Setup | `/setup` | Company setup form (shown once) |
| Dashboard | `/dashboard` | Main metrics view |
| AskCFO | `/ask` | Q&A interface with AI |

### Components

| Component | Props | Description |
|-----------|-------|-------------|
| `MetricCard` | `title, value, trend, icon, color` | Displays a single metric |
| `DocumentTable` | `documents` | Table of uploaded documents |
| `UploadBox` | `onComplete` | File upload with drag-and-drop |

### API Client (`api.js`)

```javascript
// Available functions
checkHealth()           // GET /health
setupCompany(data)      // POST /setup/company
getCompanyStatus()      // GET /company/status
uploadDocument(file, docType, periodStart, periodEnd)  // POST /documents/upload
getDocuments()          // GET /documents
getMetrics()            // GET /metrics/current
askCFO(question)        // POST /ask
```

---

## Data Models

### Company

```go
type Company struct {
    Name           string    `json:"name"`
    Industry       string    `json:"industry"`
    FiscalYearEnd  string    `json:"fiscal_year_end"`
    Currency       string    `json:"currency"`
    SetupCompleted bool      `json:"setup_completed"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}
```

### Document

```go
type Document struct {
    ID          string    `json:"id"`
    Filename    string    `json:"filename"`
    DocType     DocType   `json:"doc_type"`      // P&L, BalanceSheet, CashFlow
    PeriodStart string    `json:"period_start"`  // YYYY-MM-DD
    PeriodEnd   string    `json:"period_end"`    // YYYY-MM-DD
    FilePath    string    `json:"file_path"`
    ParsedPath  string    `json:"parsed_path"`
    UploadedAt  time.Time `json:"uploaded_at"`
    FileSize    int64     `json:"file_size"`
}
```

### ParsedDocument

```go
type ParsedDocument struct {
    DocumentID string             `json:"document_id"`
    DocType    DocType            `json:"doc_type"`
    Period     Period             `json:"period"`
    Data       map[string]float64 `json:"data"`      // Extracted metrics
    RawText    string             `json:"raw_text"`  // For RAG
    Metadata   map[string]any     `json:"metadata"`
    ParsedAt   time.Time          `json:"parsed_at"`
}
```

### FinancialMetrics

```go
type FinancialMetrics struct {
    Cash           *float64   `json:"cash"`
    MonthlyBurn    *float64   `json:"monthly_burn"`
    RunwayMonths   *float64   `json:"runway_months"`
    Revenue        *float64   `json:"revenue"`
    Expenses       *float64   `json:"expenses"`
    NetIncome      *float64   `json:"net_income"`
    TotalAssets    *float64   `json:"total_assets"`
    TotalLiab      *float64   `json:"total_liabilities"`
    Equity         *float64   `json:"equity"`
    Trends         *TrendData `json:"trends"`
    Errors         []string   `json:"errors"`
    DataSources    []string   `json:"data_sources"`
}
```

---

## Services

### Document Parser

Parses CSV, PDF, XLSX files to extract financial metrics.

**Supported Labels:**
- Revenue: "revenue", "total revenue", "sales", "net sales"
- Expenses: "expenses", "total expenses", "operating expenses"
- Cash: "cash", "cash and cash equivalents"
- Net Income: "net income", "net profit", "earnings"
- Assets/Liabilities/Equity

**CSV Format:**
```csv
Metric,Value
Revenue,1000000
Expenses,800000
Cash,500000
```

### Financial Logic

Deterministic calculations (NOT LLM):

| Function | Formula |
|----------|---------|
| `CalculateCash()` | Extract from data |
| `CalculateMonthlyBurn()` | `Expenses - Revenue` |
| `CalculateRunway()` | `Cash / Burn` (or 999 if profitable) |
| `ComparePeriods()` | `(Current - Previous) / Previous * 100` |

### RAG Service

Simple keyword-based retrieval:

1. Extract keywords from question
2. Remove stop words
3. Score documents by keyword matches
4. Extract relevant chunks
5. Return context for LLM

### LLM Service

Ollama integration:

- Builds prompts with numbers + context
- Calls Ollama `/api/generate`
- Parses SUMMARY/EXPLANATION response
- Handles errors gracefully

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Backend server port |
| `DATA_DIR` | `./data` | Data storage directory |
| `OLLAMA_HOST` | `http://ollama:11434` | Ollama API URL |
| `MODEL_NAME` | `llama3` | LLM model to use |

### Frontend Environment

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKEND_URL` | `http://0.0.0.0:8080` | Backend API URL |

---

## Deployment

### Docker Compose

```bash
# Start all services
docker compose up --build

# Pull LLM model
docker exec -it cfo-ollama ollama pull llama3

# Access
# Frontend: http://0.0.0.0:3000
# Backend: http://0.0.0.0:8080
# Ollama: http://0.0.0.0:11434
```

### Local Development

```bash
# Backend
cd backend
go run cmd/server/main.go

# Frontend
cd frontend
npm install
npm run dev

# Ollama
ollama serve
ollama pull llama3
```

---

## Development Guide

### Running Tests

```bash
# Unit tests
cd backend
go test ./internal/... -v -cover

# Integration tests
go test -tags=integration -v

# E2E tests (requires running backend)
go test -tags=e2e -v
```

### Adding a New Endpoint

1. Create handler in `internal/api/`
2. Add route in `setup.go`
3. Create service if needed in `internal/service/`
4. Add tests
5. Update frontend `api.js`

### Adding a New Financial Metric

1. Add field to `model/metrics.go`
2. Add calculation in `service/financial_logic.go`
3. Add to `FormatMetricsForPrompt()`
4. Add tests
5. Update frontend MetricCard

---

## License

Hackathon project for demonstration purposes only.

