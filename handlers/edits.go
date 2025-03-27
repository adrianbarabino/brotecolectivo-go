package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Edit struct {
	ID         int             `json:"id"`
	UserID     int             `json:"user_id"`
	EntityType string          `json:"entity_type"`
	EntityID   int             `json:"entity_id"`
	Changes    json.RawMessage `json:"changes"`
	CreatedAt  string          `json:"created_at"`
}

func (h *AuthHandler) GetEdits(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Select(`SELECT id, user_id, entity_type, entity_id, changes, created_at FROM edits ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var edits []Edit
	for rows.Next() {
		var e Edit
		var changesRaw []byte
		if err := rows.Scan(&e.ID, &e.UserID, &e.EntityType, &e.EntityID, &changesRaw, &e.CreatedAt); err == nil {
			e.Changes = changesRaw
			edits = append(edits, e)
		}
	}
	json.NewEncoder(w).Encode(edits)
}

func (h *AuthHandler) CreateEdit(w http.ResponseWriter, r *http.Request) {
	var e Edit
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, "Error al decodificar", http.StatusBadRequest)
		return
	}
	_, err := h.DB.Insert(false, `
		INSERT INTO edits (user_id, entity_type, entity_id, changes)
		VALUES (?, ?, ?, ?)`, e.UserID, e.EntityType, e.EntityID, string(e.Changes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// GetEditByID
func (h *AuthHandler) GetEditByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var e Edit
	var changesRaw []byte

	row, err := h.DB.SelectRow("SELECT id, user_id, entity_type, entity_id, changes, created_at FROM edits WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = row.Scan(&e.ID, &e.UserID, &e.EntityType, &e.EntityID, &changesRaw, &e.CreatedAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	e.Changes = changesRaw
	json.NewEncoder(w).Encode(e)
}

// UpdateEditStatus
func (h *AuthHandler) UpdateEditStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		Status     string `json:"status"`
		Comment    string `json:"comment"`
		ReviewerID int    `json:"reviewer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Error al decodificar", http.StatusBadRequest)
		return
	}
	_, err := h.DB.Update(false, `
		UPDATE edits SET status=?, comment=?, reviewed_by=? WHERE id=?`,
		payload.Status, payload.Comment, payload.ReviewerID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
