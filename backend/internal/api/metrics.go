package api

import (
	"net/http"

	"github.com/cfo/backend/internal/service"
	"github.com/cfo/backend/internal/storage"
)

// MetricsHandler handles metrics endpoints
type MetricsHandler struct {
	store    *storage.FileStore
	finLogic *service.FinancialLogic
}

// NewMetricsHandler creates a new MetricsHandler
func NewMetricsHandler(store *storage.FileStore) *MetricsHandler {
	return &MetricsHandler{
		store:    store,
		finLogic: service.NewFinancialLogic(store),
	}
}

// GetCurrent handles GET /metrics/current
func (h *MetricsHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics, err := h.finLogic.CalculateCurrentMetrics()
	if err != nil {
		writeError(w, "Failed to calculate metrics: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, metrics)
}

