package handlers

import (
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
	"github.com/jmoiron/sqlx"
)

// Handler holds all handlers for the API
type Handler struct {
	DB             *sqlx.DB
	DerivedCatalog models.DerivedMetricsCatalog
}

// NewHandler creates a new Handler
func NewHandler(db *sqlx.DB, derivedCatalog models.DerivedMetricsCatalog) *Handler {
	return &Handler{
		DB:             db,
		DerivedCatalog: derivedCatalog,
	}
}
