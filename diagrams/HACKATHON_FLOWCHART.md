# AI CFO - Hackathon Presentation Flowchart

## 🎯 System Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           AI CFO - ON-PREMISE SOLUTION                          │
│                    "Your Financial Intelligence, Your Infrastructure"            │
└─────────────────────────────────────────────────────────────────────────────────┘

    ┌──────────────┐         ┌──────────────┐         ┌──────────────┐
    │   Upload     │         │     Ask      │         │   Dashboard  │
    │  Documents   │         │   Questions  │         │    View      │
    │  (PDF/CSV)   │         │  (Natural)   │         │   Metrics    │
    └──────┬───────┘         └──────┬───────┘         └──────┬───────┘
           │                        │                        │
           └────────────────────────┼────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              REACT FRONTEND                                      │
│                         (Beautiful, Modern UI)                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              NGINX PROXY                                         │
│                     (Secure routing, no external calls)                          │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              GO BACKEND                                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            │
│  │   Document  │  │  Financial  │  │     RAG     │  │     LLM     │            │
│  │   Parser    │  │    Logic    │  │   Search    │  │   Service   │            │
│  │ (PDF/XLSX)  │  │ (GO CODE)   │  │  (Context)  │  │  (Explain)  │            │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘            │
└─────────────────────────────────────────────────────────────────────────────────┘
           │                │                │                │
           ▼                ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           LOCAL FILE STORAGE                                     │
│              (JSON files - No database, simple & portable)                       │
└─────────────────────────────────────────────────────────────────────────────────┘
                                                              │
                                                              ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         LOCAL LLM (OLLAMA + LLAMA3)                              │
│                    🔒 100% ON-PREMISE - NO CLOUD CALLS 🔒                        │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 🔄 User Journey Flow

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              USER JOURNEY                                        │
└─────────────────────────────────────────────────────────────────────────────────┘

    ┌─────────┐      ┌─────────┐      ┌─────────┐      ┌─────────┐      ┌─────────┐
    │  STEP 1 │ ───► │  STEP 2 │ ───► │  STEP 3 │ ───► │  STEP 4 │ ───► │  STEP 5 │
    │ ONBOARD │      │ UPLOAD  │      │  VIEW   │      │   ASK   │      │ DECIDE  │
    └─────────┘      └─────────┘      └─────────┘      └─────────┘      └─────────┘
         │                │                │                │                │
         ▼                ▼                ▼                ▼                ▼
    ┌─────────┐      ┌─────────┐      ┌─────────┐      ┌─────────┐      ┌─────────┐
    │ Company │      │ Balance │      │  Cash   │      │ "What's │      │  Make   │
    │  Setup  │      │ Sheets  │      │ Revenue │      │   my    │      │ Smarter │
    │  Info   │      │  P&L    │      │ Runway  │      │ profit?"│      │ Choices │
    └─────────┘      └─────────┘      └─────────┘      └─────────┘      └─────────┘
```

---

## 💡 "Ask CFO" Processing Flow

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         HOW "ASK CFO" WORKS                                      │
└─────────────────────────────────────────────────────────────────────────────────┘

    USER QUESTION: "What's my profit in Q4 2024?"
                              │
                              ▼
                   ┌─────────────────────┐
                   │   PERIOD PARSER     │
                   │   "Q4 2024" →       │
                   │   Oct-Dec 2024      │
                   └──────────┬──────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
    ┌─────────────────┐ ┌─────────────┐ ┌─────────────────┐
    │  FINANCIAL      │ │    RAG      │ │   LOAD          │
    │  LOGIC (GO)     │ │   SEARCH    │ │   DOCUMENTS     │
    │                 │ │             │ │                 │
    │ Calculate:      │ │ Find chunks │ │ Filter by       │
    │ • Net Income    │ │ matching    │ │ Q4 2024 period  │
    │ • Cash          │ │ "profit"    │ │                 │
    │ • Runway        │ │ "Q4"        │ │                 │
    └────────┬────────┘ └──────┬──────┘ └────────┬────────┘
             │                 │                 │
             └─────────────────┼─────────────────┘
                               │
                               ▼
                   ┌─────────────────────┐
                   │   BUILD LLM PROMPT  │
                   │                     │
                   │ • Calculated numbers│
                   │ • Document context  │
                   │ • User question     │
                   └──────────┬──────────┘
                              │
                              ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │                     LOCAL LLM (LLAMA3)                          │
    │                                                                 │
    │   📝 PROMPT:                                                    │
    │   "You are an AI CFO. Here are the CALCULATED metrics:          │
    │    Net Income: $804,000 | Period: Q4 2024                       │
    │    Document context: [relevant excerpts]                        │
    │    EXPLAIN these numbers, don't calculate."                     │
    │                                                                 │
    │   🤖 RESPONSE:                                                  │
    │   "SUMMARY: Your Q4 2024 profit was $804,000..."                │
    └─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
                   ┌─────────────────────┐
                   │   PARSE RESPONSE    │
                   │   • Extract Summary │
                   │   • Extract Detail  │
                   │   • Add Sources     │
                   └──────────┬──────────┘
                              │
                              ▼
    ┌─────────────────────────────────────────────────────────────────┐
    │                      FINAL RESPONSE                              │
    │                                                                  │
    │   ✅ Summary: "Your Q4 2024 profit was $804,000"                │
    │   📊 Numbers: Cash, Revenue, Net Income, Runway                 │
    │   📄 Sources: doc_123, doc_456 (original files)                 │
    │   📝 Explanation: Detailed AI-generated context                 │
    └─────────────────────────────────────────────────────────────────┘
```

---

## 🔒 Security Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           SECURITY FLOW                                          │
└─────────────────────────────────────────────────────────────────────────────────┘

     YOUR NETWORK                           │            INTERNET
     (100% Local)                           │           (BLOCKED)
                                            │
    ┌──────────────┐                        │         ╔═══════════════╗
    │    User      │                        │         ║   OpenAI      ║
    │   Browser    │                        │    ❌   ║   Claude      ║
    └──────┬───────┘                        │   ───►  ║   Cloud LLMs  ║
           │                                │         ╚═══════════════╝
           ▼                                │
    ┌──────────────┐                        │
    │   Frontend   │                        │
    │   :3000      │                        │
    └──────┬───────┘                        │
           │                                │
           ▼                                │
    ┌──────────────┐                        │
    │   Backend    │                        │
    │   :8080      │                        │
    └──────┬───────┘                        │
           │                                │
           ▼                                │
    ┌──────────────┐                        │
    │   Ollama     │◄── Local LLM           │
    │   (llama3)   │    No API Keys         │
    │   :11434     │    No Data Leakage     │
    └──────────────┘                        │
           │                                │
           ▼                                │
    ┌──────────────┐                        │
    │  Local Disk  │◄── File Storage        │
    │   ./data/    │    No Cloud DB         │
    └──────────────┘                        │
                                            │
    ════════════════════════════════════════╧════════════════════════════════

              🔒 YOUR DATA NEVER LEAVES YOUR INFRASTRUCTURE 🔒
```

---

## 📊 Value Proposition

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           BEFORE vs AFTER                                        │
└─────────────────────────────────────────────────────────────────────────────────┘

     ❌ BEFORE AI CFO                           ✅ AFTER AI CFO
    ─────────────────                          ─────────────────

    ┌──────────────────┐                       ┌──────────────────┐
    │ Hire CFO/Analyst │                       │ Upload Documents │
    │    $150K/year    │                       │      FREE        │
    └────────┬─────────┘                       └────────┬─────────┘
             │                                          │
             ▼                                          ▼
    ┌──────────────────┐                       ┌──────────────────┐
    │  Manual Analysis │                       │  Instant Metrics │
    │    Hours/Days    │                       │     Seconds      │
    └────────┬─────────┘                       └────────┬─────────┘
             │                                          │
             ▼                                          ▼
    ┌──────────────────┐                       ┌──────────────────┐
    │ Spreadsheet Hell │                       │ Natural Language │
    │  Complex Formulas│                       │   "Ask CFO"      │
    └────────┬─────────┘                       └────────┬─────────┘
             │                                          │
             ▼                                          ▼
    ┌──────────────────┐                       ┌──────────────────┐
    │  Cloud Services  │                       │  100% On-Premise │
    │   Data at Risk   │                       │   Your Control   │
    └────────┬─────────┘                       └────────┬─────────┘
             │                                          │
             ▼                                          ▼
    ┌──────────────────┐                       ┌──────────────────┐
    │  Monthly Reports │                       │  Real-Time       │
    │  Delayed Insight │                       │  Decisions       │
    └──────────────────┘                       └──────────────────┘
```

---

## 🚀 Tech Stack

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              TECH STACK                                          │
└─────────────────────────────────────────────────────────────────────────────────┘

    FRONTEND                BACKEND                 AI/ML                INFRA
    ────────                ───────                 ─────                ─────
    
    ┌────────┐             ┌────────┐             ┌────────┐          ┌────────┐
    │ React  │             │   Go   │             │ Ollama │          │ Docker │
    │ + Vite │             │ 1.21   │             │  LLM   │          │Compose │
    └────────┘             └────────┘             └────────┘          └────────┘
    
    ┌────────┐             ┌────────┐             ┌────────┐          ┌────────┐
    │  CSS   │             │  HTTP  │             │ llama3 │          │ Nginx  │
    │ Modern │             │  REST  │             │  8B    │          │ Proxy  │
    └────────┘             └────────┘             └────────┘          └────────┘
    
    ┌────────┐             ┌────────┐             ┌────────┐          ┌────────┐
    │  SPA   │             │  JSON  │             │Keyword │          │ Local  │
    │ Router │             │ Files  │             │  RAG   │          │Storage │
    └────────┘             └────────┘             └────────┘          └────────┘
```

---

## 📋 Key Differentiators

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         WHY WE'RE DIFFERENT                                      │
└─────────────────────────────────────────────────────────────────────────────────┘

    1️⃣  ON-PREMISE LLM
        ───────────────
        • No OpenAI API keys
        • No data sent to cloud
        • Works offline

    2️⃣  DETERMINISTIC CALCULATIONS
        ──────────────────────────
        • Math done in Go code
        • LLM only EXPLAINS
        • No hallucinated numbers

    3️⃣  NATURAL LANGUAGE QUERIES
        ─────────────────────────
        • "What's my Q4 profit?"
        • "How long is our runway?"
        • Period detection built-in

    4️⃣  DOCUMENT INTELLIGENCE
        ─────────────────────
        • PDF, CSV, XLSX support
        • Multi-page parsing
        • Context-aware RAG

    5️⃣  ZERO DEPENDENCIES
        ─────────────────
        • Docker Compose only
        • No cloud subscriptions
        • Single command deploy
```

---

## 🎯 One Command to Run

```bash
docker-compose up -d
# Open http://localhost:3000
# That's it! 🎉
```

