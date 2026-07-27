// Package handler contains HTTP handlers. Handlers translate between HTTP
// and the service layer — they contain no business logic and never talk to
// Redis or PostgreSQL directly.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/progspac-vnn/CachingStampede/internal/model"
	"github.com/progspac-vnn/CachingStampede/internal/service"
)

// ProductService is the subset of *service.ProductService this handler
// depends on, defined here so it can be mocked in tests.
type ProductService interface {
	GetProduct(ctx context.Context, id int64) (*model.Product, error)
}

// ProductHandler handles HTTP requests for products.
type ProductHandler struct {
	service ProductService
	log     *zap.Logger
}

// NewProductHandler constructs a ProductHandler.
func NewProductHandler(service ProductService, log *zap.Logger) *ProductHandler {
	return &ProductHandler{service: service, log: log}
}

// GetProduct handles GET /products/{id}.
func (h *ProductHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	product, err := h.service.GetProduct(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			writeError(w, http.StatusNotFound, "product not found")
			return
		}
		h.log.Error("failed to get product", zap.Int64("product_id", id), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, product)
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
