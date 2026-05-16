# AI CFO - Sequence Diagrams

Use these diagrams at https://sequencediagram.org or any Mermaid-compatible viewer.

---

## 1. Application Startup Flow

```mermaid
sequenceDiagram
    participant User
    participant Browser
    participant Frontend as Frontend (React)
    participant Backend as Backend (Go)
    participant Storage as File Storage
    participant Ollama as Ollama LLM

    User->>Browser: Open http://0.0.0.0:3000
    Browser->>Frontend: Load React App
    Frontend->>Backend: GET /company/status
    Backend->>Storage: Read company.json
    Storage-->>Backend: Company data (or null)
    Backend-->>Frontend: {setup_completed: false/true}
    
    alt Setup not completed
        Frontend-->>Browser: Show Setup Page
    else Setup completed
        Frontend-->>Browser: Show Dashboard
    end
```

---

## 2. Company Setup Flow

```mermaid
sequenceDiagram
    participant User
    participant Frontend as Frontend (React)
    participant Backend as Backend (Go)
    participant Storage as File Storage

    User->>Frontend: Enter company details
    User->>Frontend: Click "Continue to Dashboard"
    Frontend->>Backend: POST /setup/company
    Note right of Frontend: {name, industry, fiscal_year_end, currency}
    
    Backend->>Backend: Validate request
    alt Validation failed
        Backend-->>Frontend: 400 Bad Request
        Frontend-->>User: Show error message
    else Validation passed
        Backend->>Storage: Write company.json
        Storage-->>Backend: Success
        Backend-->>Frontend: 200 OK + Company data
        Frontend-->>User: Redirect to Dashboard
    end
```

---

## 3. Document Upload Flow

```mermaid
sequenceDiagram
    participant User
    participant Frontend as Frontend (React)
    participant Backend as Backend (Go)
    participant Parser as Document Parser
    participant Storage as File Storage

    User->>Frontend: Select file (CSV/PDF/XLSX)
    User->>Frontend: Set doc_type, period
    User->>Frontend: Click Upload
    
    Frontend->>Backend: POST /documents/upload
    Note right of Frontend: multipart/form-data
    
    Backend->>Backend: Validate file type
    alt Invalid file type
        Backend-->>Frontend: 400 Bad Request
        Frontend-->>User: Show error
    else Valid file type
        Backend->>Storage: Save raw file to /data/documents/
        Storage-->>Backend: File path
        
        Backend->>Parser: Parse document
        Parser->>Parser: Extract financial metrics
        Note right of Parser: Revenue, Expenses, Cash, etc.
        Parser-->>Backend: Parsed data
        
        Backend->>Storage: Save parsed JSON to /data/parsed/
        Backend->>Storage: Update documents.json
        Storage-->>Backend: Success
        
        Backend-->>Frontend: 200 OK + Document metadata
        Frontend-->>User: Show success, update list
    end
```

---

## 4. Dashboard Metrics Flow

```mermaid
sequenceDiagram
    participant User
    participant Frontend as Frontend (React)
    participant Backend as Backend (Go)
    participant FinLogic as Financial Logic
    participant Storage as File Storage

    User->>Frontend: View Dashboard
    Frontend->>Backend: GET /metrics/current
    
    Backend->>Storage: Load all parsed/*.json
    Storage-->>Backend: Array of parsed documents
    
    Backend->>FinLogic: CalculateCurrentMetrics()
    
    FinLogic->>FinLogic: Aggregate latest data
    FinLogic->>FinLogic: CalculateCash()
    FinLogic->>FinLogic: CalculateMonthlyBurn()
    Note right of FinLogic: Burn = Expenses - Revenue
    FinLogic->>FinLogic: CalculateRunway()
    Note right of FinLogic: Runway = Cash / Burn
    FinLogic->>FinLogic: ComparePeriods()
    Note right of FinLogic: Calculate % changes
    
    FinLogic-->>Backend: FinancialMetrics
    Backend-->>Frontend: JSON metrics
    
    Frontend->>Frontend: Render MetricCards
    Frontend-->>User: Display Dashboard
```

---

## 5. Ask CFO Flow (Main Feature)

```mermaid
sequenceDiagram
    participant User
    participant Frontend as Frontend (React)
    participant Backend as Backend (Go)
    participant FinLogic as Financial Logic
    participant RAG as RAG Service
    participant LLM as Ollama LLM
    participant Storage as File Storage

    User->>Frontend: Enter question
    User->>Frontend: Click Submit
    Frontend->>Backend: POST /ask
    Note right of Frontend: {question: "What is our cash position?"}
    
    %% Step 1: Calculate metrics (CODE, not LLM)
    Backend->>FinLogic: CalculateCurrentMetrics()
    FinLogic->>Storage: Load parsed documents
    Storage-->>FinLogic: Document data
    FinLogic->>FinLogic: Calculate all metrics
    FinLogic-->>Backend: Metrics (cash, burn, runway, etc.)
    
    %% Step 2: RAG retrieval
    Backend->>RAG: Search(question)
    RAG->>Storage: Load all parsed documents
    RAG->>RAG: Extract keywords
    RAG->>RAG: Score documents
    RAG->>RAG: Extract relevant chunks
    RAG-->>Backend: Context + Source IDs
    
    %% Step 3: Format for LLM
    Backend->>FinLogic: FormatMetricsForPrompt()
    FinLogic-->>Backend: ["Cash: $500K", "Burn: $50K", ...]
    
    %% Step 4: LLM explanation (EXPLAIN only, not calculate)
    Backend->>LLM: POST /api/generate
    Note right of Backend: Prompt with numbers + context
    LLM->>LLM: Generate explanation
    LLM-->>Backend: {response: "SUMMARY:... EXPLANATION:..."}
    
    %% Step 5: Parse and return
    Backend->>Backend: Parse LLM response
    Backend-->>Frontend: AskResponse
    Note left of Backend: {summary, numbers_used, explanation, sources}
    
    Frontend->>Frontend: Render response cards
    Frontend-->>User: Display AI CFO answer
```

---

## 6. Data Persistence Flow

```mermaid
sequenceDiagram
    participant Backend as Backend (Go)
    participant FileStore as FileStore
    participant Disk as File System

    Note over Backend,Disk: Save Company
    Backend->>FileStore: SaveCompany(company)
    FileStore->>FileStore: Set UpdatedAt = now
    FileStore->>Disk: Write /data/state/company.json
    Disk-->>FileStore: Success
    FileStore-->>Backend: nil (no error)
    
    Note over Backend,Disk: Save Document Metadata
    Backend->>FileStore: SaveDocumentList(docs)
    FileStore->>Disk: Write /data/state/documents.json
    
    Note over Backend,Disk: Save Parsed Document
    Backend->>FileStore: SaveParsedDocument(parsed)
    FileStore->>Disk: Write /data/parsed/{docID}.json
    
    Note over Backend,Disk: Load All Parsed Documents
    Backend->>FileStore: LoadAllParsedDocuments()
    FileStore->>Disk: ReadDir /data/parsed/
    Disk-->>FileStore: List of .json files
    loop For each file
        FileStore->>Disk: Read file
        Disk-->>FileStore: JSON content
        FileStore->>FileStore: Decode to ParsedDocument
    end
    FileStore-->>Backend: []*ParsedDocument
```

---

## 7. Error Handling Flow

```mermaid
sequenceDiagram
    participant User
    participant Frontend as Frontend (React)
    participant Backend as Backend (Go)
    participant LLM as Ollama LLM

    User->>Frontend: Ask question
    Frontend->>Backend: POST /ask
    
    Backend->>LLM: POST /api/generate
    
    alt LLM not available
        LLM-->>Backend: Connection refused
        Backend->>Backend: Create error response
        Backend-->>Frontend: {summary: "", numbers_used: [...], error: "LLM unavailable"}
        Frontend-->>User: Show numbers without explanation
    else LLM timeout
        LLM-->>Backend: Timeout
        Backend-->>Frontend: {error: "LLM timeout"}
    else LLM success
        LLM-->>Backend: Response
        Backend-->>Frontend: Full response with explanation
        Frontend-->>User: Show complete answer
    end
```

---

## 8. Complete System Architecture

```mermaid
flowchart TB
    subgraph Client["Client (Browser)"]
        UI[React Frontend]
    end
    
    subgraph Server["Backend Server"]
        API[HTTP API Layer]
        Services[Services Layer]
        Storage[Storage Layer]
    end
    
    subgraph Services
        Parser[Document Parser]
        FinLogic[Financial Logic]
        RAG[RAG Service]
        LLMSvc[LLM Service]
    end
    
    subgraph External["External Services"]
        Ollama[Ollama LLM]
    end
    
    subgraph DataStore["File Storage"]
        Docs[/data/documents/]
        Parsed[/data/parsed/]
        State[/data/state/]
    end
    
    UI -->|HTTP| API
    API --> Parser
    API --> FinLogic
    API --> RAG
    API --> LLMSvc
    
    Parser --> Storage
    FinLogic --> Storage
    RAG --> Storage
    LLMSvc -->|HTTP| Ollama
    
    Storage --> Docs
    Storage --> Parsed
    Storage --> State
```

---

## 9. Financial Calculations Flow

```mermaid
flowchart LR
    subgraph Input["Input Data"]
        Rev[Revenue]
        Exp[Expenses]
        Cash[Cash]
    end
    
    subgraph Calculations["Deterministic Calculations"]
        Burn["Monthly Burn<br/>= Expenses - Revenue"]
        Runway["Runway<br/>= Cash / Burn"]
        NetIncome["Net Income<br/>= Revenue - Expenses"]
    end
    
    subgraph Output["Output Metrics"]
        CashOut[Cash Position]
        BurnOut[Monthly Burn Rate]
        RunwayOut[Runway Months]
        Trends[Period Trends]
    end
    
    Rev --> Burn
    Exp --> Burn
    Cash --> CashOut
    Burn --> BurnOut
    Cash --> Runway
    Burn --> Runway
    Runway --> RunwayOut
    
    Rev --> NetIncome
    Exp --> NetIncome
    
    style Calculations fill:#e1f5fe
    style Input fill:#fff3e0
    style Output fill:#e8f5e9
```

---

## 10. Docker Deployment Flow

```mermaid
sequenceDiagram
    participant Dev as Developer
    participant Docker as Docker Compose
    participant Ollama as Ollama Container
    participant Backend as Backend Container
    participant Frontend as Frontend Container

    Dev->>Docker: docker compose up --build
    
    Docker->>Ollama: Start ollama service
    Ollama-->>Docker: Ready on :11434
    
    Docker->>Backend: Build Go binary
    Docker->>Backend: Start backend service
    Backend->>Backend: Initialize /app/data directories
    Backend-->>Docker: Ready on :8080
    
    Docker->>Frontend: Build React app
    Docker->>Frontend: Start nginx
    Frontend-->>Docker: Ready on :3000
    
    Note over Docker: All services running
    
    Dev->>Ollama: ollama pull llama3
    Ollama->>Ollama: Download model
    Ollama-->>Dev: Model ready
    
    Dev->>Frontend: Open http://0.0.0.0:3000
    Frontend->>Backend: API calls via /api proxy
    Backend->>Ollama: LLM requests
```

---

## Usage Instructions

### Option 1: SequenceDiagram.org
1. Go to https://sequencediagram.org
2. Copy the content between ```mermaid and ``` 
3. Paste and render

### Option 2: Mermaid Live Editor
1. Go to https://mermaid.live
2. Paste the Mermaid code
3. Export as PNG/SVG

### Option 3: VS Code
1. Install "Mermaid Preview" extension
2. Open this file
3. Preview renders automatically

### Option 4: GitHub/GitLab
- These diagrams render automatically in markdown files

