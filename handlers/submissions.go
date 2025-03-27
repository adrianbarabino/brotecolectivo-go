package handlers

import (
	"brotecolectivo/models"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Submission struct {
	ID        int             `json:"id"`
	UserID    int             `json:"user_id"`
	Type      string          `json:"type"` // ejemplo: "banda", "cancion", etc.
	Data      json.RawMessage `json:"data"`
	Status    string          `json:"status"` // pendiente, aprobado, rechazado
	Reviewer  *models.User    `json:"reviewer,omitempty"`
	Comment   string          `json:"comment"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func (h *AuthHandler) GetSubmissions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Select(`
		SELECT s.id, s.user_id, s.type, s.data, s.status, s.comment, s.created_at, s.updated_at,
		       u.id, u.name, u.email
		FROM submissions s
		LEFT JOIN users u ON s.reviewed_by = u.id
		ORDER BY s.created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var subs []Submission
	for rows.Next() {
		var s Submission
		var dataRaw []byte
		var reviewerID sql.NullInt64
		var reviewerName, reviewerEmail sql.NullString

		err := rows.Scan(&s.ID, &s.UserID, &s.Type, &dataRaw, &s.Status, &s.Comment, &s.CreatedAt, &s.UpdatedAt,
			&reviewerID, &reviewerName, &reviewerEmail)
		if err != nil {
			continue
		}
		s.Data = dataRaw
		if reviewerID.Valid {
			s.Reviewer = &models.User{
				ID:    int(reviewerID.Int64),
				Name:  reviewerName.String,
				Email: reviewerEmail.String,
			}
		}
		subs = append(subs, s)
	}
	json.NewEncoder(w).Encode(subs)
}

func (h *AuthHandler) GetSubmissionByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := h.DB.SelectRow(`
		SELECT id, user_id, type, data, status, comment, created_at, updated_at
		FROM submissions WHERE id = ?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var s Submission
	var dataRaw []byte
	err = row.Scan(&s.ID, &s.UserID, &s.Type, &dataRaw, &s.Status, &s.Comment, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		http.Error(w, "No encontrado", http.StatusNotFound)
		return
	}
	s.Data = dataRaw
	json.NewEncoder(w).Encode(s)
}

func (h *AuthHandler) CreateSubmission(w http.ResponseWriter, r *http.Request) {
	var s Submission
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "Error al decodificar", http.StatusBadRequest)
		return
	}
	id, err := h.DB.Insert(false, `
		INSERT INTO submissions (user_id, type, data, status)
		VALUES (?, ?, ?, 'pendiente')`,
		s.UserID, s.Type, string(s.Data))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.ID = int(id)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(s)
}

func (h *AuthHandler) UpdateSubmissionStatus(w http.ResponseWriter, r *http.Request) {
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
		UPDATE submissions SET status=?, comment=?, reviewed_by=? WHERE id=?`,
		payload.Status, payload.Comment, payload.ReviewerID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
