package handlers

import (
	"github.com/jmoiron/sqlx"
	"github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/models"
)

// Handler enthält alle Handler für die API
type Handler struct {
	DB             *sqlx.DB
	DerivedCatalog models.DerivedMetricsCatalog
}

// NewHandler erstellt einen neuen Handler
func NewHandler(db *sqlx.DB, derivedCatalog models.DerivedMetricsCatalog) *Handler {
	return &Handler{
		DB:             db,
		DerivedCatalog: derivedCatalog,
	}
}
