package handlers

import (
	"github.com/jmoiron/sqlx"
)

// Handler enthält alle Handler für die API
type Handler struct {
	DB *sqlx.DB
}

// NewHandler erstellt einen neuen Handler
func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{
		DB: db,
	}
}
