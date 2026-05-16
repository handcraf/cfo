# AI CFO - Project Summary

## 🎯 What We Built

An **on-premises AI-powered CFO** that:
- ✅ Understands a company's financial history
- ✅ Tracks changes over time
- ✅ Explains business decisions using local LLM
- ✅ **Never sends data to the cloud**

---

## 🏗️ Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                         BROWSER                               │
│  ┌────────────────────────────────────────────────────────┐  │
│  │           React Frontend (Vite + nginx)                 │  │
│  │   http://0.0.0.0:3000                                 │  │
│  └─────────────────────────┬──────────────────────────────┘  │
└─────────────────────────────┼────────────────────────────────┘
                              │ /api/* proxy
┌─────────────────────────────┼────────────────────────────────┐
│                          BACKEND                              │
│  ┌──────────────────────────┴─────────────────────────────┐  │
│  │            Go HTTP Server (Gorilla Mux)                 │  │
│  │   http://0.0.0.0:8080                                 │  │
│  └──────────────────────────┬─────────────────────────────┘  │
│                             │                                 │
│  ┌──────────────────────────┴─────────────────────────────┐  │
│  │                    SERVICES                             │  │
│  │  ┌─────────┐  ┌─────────┐  ┌─────┐  ┌─────────────────┐│  │
│  │  │ Parser  │  │Finance  │  │ RAG │  │  LLM Service    ││  │
│  │  │(CSV/PDF)│  │ Logic   │  │     │  │  (explains)     ││  │
│  │  └─────────┘  └─────────┘  └─────┘  └────────┬────────┘│  │
│  └──────────────────────────────────────────────┼─────────┘  │
└─────────────────────────────────────────────────┼────────────┘
                                                  │
┌─────────────────────────────────────────────────┼────────────┐
│                       OLLAMA LLM                │            │
│  ┌──────────────────────────────────────────────┴─────────┐  │
│  │       tinyllama / llama3 (local model)                  │  │
│  │   http://0.0.0.0:11434                                │  │
│  └─────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

---

## 💡 Key Design Principles

### 1. LLM Explains, Code Calculates

```
Financial Metrics (Code):          LLM Explanations:
├── Cash Position     ──────────►  "Your cash of $500K..."
├── Monthly Burn Rate ──────────►  "You're burning $50K/month..."
├── Runway (months)   ──────────►  "This gives 10 months runway..."
└── Period Trends     ──────────►  "Revenue grew 20% vs Q1..."
```

**Why?** LLMs can hallucinate numbers. Financial calculations must be deterministic.

### 2. No Database, Files Only

```
/data
├── documents/      # Raw uploaded files (CSV, PDF, XLSX)
├── parsed/         # Extracted metrics (JSON)
└── state/          # App state (company.json, documents.json)
```

**Why?** Easy to backup, inspect, debug. No database setup required.

### 3. Keyword-Based RAG (Simple & Effective)

```
Question: "What is our cash position?"
           ↓
Keywords: ["cash", "position"]
           ↓
Search: Score each document by keyword matches
           ↓
Context: "Revenue: $1M, Cash: $500K, Expenses: $800K"
           ↓
LLM: Generate explanation with context
```

**Why?** Works offline, no embeddings API needed.

---

## 🔧 Technology Stack

| Component | Technology | Purpose |
|-----------|------------|---------|
| Backend | Go 1.21+ | API server, business logic |
| HTTP Router | Gorilla Mux | REST API routing |
| Frontend | React 18 + Vite | User interface |
| Styling | CSS | Custom styling |
| LLM | Ollama (tinyllama) | Natural language explanations |
| Storage | JSON files | Data persistence |
| Container | Docker Compose | Deployment |

---

## 📊 Financial Metrics Calculated

| Metric | Formula | Notes |
|--------|---------|-------|
| **Cash** | Direct from data | Cash and equivalents |
| **Revenue** | Direct from data | Total revenue/sales |
| **Expenses** | Direct from data | Total operating expenses |
| **Monthly Burn** | `Expenses - Revenue` | Negative = profitable |
| **Runway** | `Cash / Burn` | 999 = infinite (profitable) |
| **Net Income** | `Revenue - Expenses` | Profit/Loss |
| **Trends** | `(Current - Previous) / Previous * 100` | % change |

---

## 🔌 API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/setup/company` | POST | Setup company info |
| `/company/status` | GET | Get setup status |
| `/documents/upload` | POST | Upload financial doc |
| `/documents` | GET | List all documents |
| `/metrics/current` | GET | Get calculated metrics |
| `/ask` | POST | Ask CFO a question |

---

## 🧪 Testing Summary

| Category | Tests | Result |
|----------|-------|--------|
| Unit Tests | 75+ | ✅ All Pass |
| Integration Tests | 12 | ✅ All Pass |
| E2E Tests | 15+ | ✅ All Pass |
| **Total** | **100+** | ✅ **All Pass** |

**Coverage:** ~78%

---

## 🚀 Quick Start

### Local Development

```bash
# Terminal 1: Ollama
ollama serve
ollama pull tinyllama

# Terminal 2: Backend
cd backend
DATA_DIR=./data OLLAMA_HOST=http://0.0.0.0:11434 MODEL_NAME=tinyllama go run cmd/server/main.go

# Terminal 3: Frontend
cd frontend
npm install
npm run dev

# Open http://0.0.0.0:3000
```

### Docker Compose

```bash
docker compose up --build
docker exec -it cfo-ollama ollama pull llama3
# Open http://0.0.0.0:3000
```

---

## 📁 Project Structure

```
/cfo
├── backend/
│   ├── cmd/server/main.go      # Entry point
│   ├── internal/
│   │   ├── api/                # HTTP handlers
│   │   ├── service/            # Business logic
│   │   ├── storage/            # File storage
│   │   ├── model/              # Data models
│   │   └── config/             # Configuration
│   ├── data/                   # Data directory
│   └── testdata/               # Test CSV files
├── frontend/
│   ├── src/
│   │   ├── pages/              # React pages
│   │   ├── components/         # UI components
│   │   └── api.js              # API client
│   └── vite.config.js          # Dev server config
├── diagrams/                   # Architecture diagrams
├── docs/                       # Documentation
├── docker-compose.yml
├── run.md                      # Setup guide
├── TESTING.md                  # Test documentation
└── PROJECT_SUMMARY.md          # This file
```

---

## 📈 Sample Flow

1. **Setup Company** → Enter name, industry, currency
2. **Upload Documents** → P&L, Balance Sheet, Cash Flow (CSV)
3. **View Dashboard** → See cash, burn, runway metrics
4. **Ask CFO** → "What is our cash position?"
5. **Get Answer** → LLM explains the numbers in plain English

---

## 🎯 Hackathon Demo Script

```
1. Open http://0.0.0.0:3000
2. "This is our AI CFO - runs entirely on-prem"
3. Show setup page: "Company setup takes 10 seconds"
4. Dashboard: "Here are our financial metrics"
5. Explain: "These numbers are calculated by code, not AI"
6. Ask CFO: "Now let's ask our AI CFO about our runway"
7. Show response: "The AI explains the numbers in plain English"
8. Highlight: "All data stays on your machine - no cloud, no API keys"
```

---

## 🏆 Key Differentiators

| Feature | AI CFO | Typical SaaS |
|---------|--------|--------------|
| Data Privacy | ✅ 100% Local | ❌ Cloud |
| Internet Required | ❌ No | ✅ Yes |
| API Keys | ❌ None | ✅ Required |
| Calculation Accuracy | ✅ Deterministic | ⚠️ LLM may hallucinate |
| Deployment | ✅ Docker Compose | ❌ Complex |

---

*Built for Hackathon 2025 - December 19, 2025*

