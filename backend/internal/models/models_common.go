package models

// APIResponse is the standard API response
type APIResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message,omitempty" example:"Operation erfolgreich"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty" example:""`
	Meta    *MetaInfo   `json:"meta,omitempty"`
}

// MetaInfo holds pagination metadata and an optional note on how the data
// was derived (e.g. which values the metrics endpoints leave out).
type MetaInfo struct {
	Total  int    `json:"total,omitempty" example:"140400"`
	Limit  int    `json:"limit,omitempty" example:"100"`
	Offset int    `json:"offset,omitempty" example:"0"`
	Note   string `json:"note,omitempty" example:"Nur gültige Werte gehen in die Statistik ein"`
}
