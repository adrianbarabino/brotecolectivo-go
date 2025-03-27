package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Event struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Tags      string `json:"tags"`
	Content   string `json:"content"`
	Slug      string `json:"slug"`
	DateStart string `json:"date_start"`
	DateEnd   string `json:"date_end"`
	Venue     *Venue `json:"venue"`
	Bands     []Band `json:"bands"`
}

func (h *AuthHandler) GetEvents(w http.ResponseWriter, r *http.Request) {
	offsetParam := r.URL.Query().Get("offset")
	limitParam := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("q")

	offset := 0
	limit := 10
	var err error

	if offsetParam != "" {
		offset, err = strconv.Atoi(offsetParam)
		if err != nil {
			http.Error(w, "Offset inválido", http.StatusBadRequest)
			return
		}
	}
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil {
			http.Error(w, "Límite inválido", http.StatusBadRequest)
			return
		}
	}

	query := `
		SELECT 
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE 1=1
	`
	var queryParams []interface{}

	if search != "" {
		query += " AND (e.title LIKE ? OR e.tags LIKE ? OR e.content LIKE ? OR e.slug LIKE ?)"
		searchPattern := "%" + search + "%"
		queryParams = append(queryParams, searchPattern, searchPattern, searchPattern, searchPattern)
	}

	query += " ORDER BY e.date_start DESC LIMIT ? OFFSET ?"
	queryParams = append(queryParams, limit, offset)

	rows, err := h.DB.Select(query, queryParams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []Event

	for rows.Next() {
		var e Event
		var v Venue
		err := rows.Scan(&e.ID, &e.Title, &e.Tags, &e.Content, &e.Slug, &e.DateStart, &e.DateEnd,
			&v.ID, &v.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		e.Venue = &v

		// Bandas asociadas
		bandRows, err := h.DB.Select(`
			SELECT b.id, b.name, b.slug
			FROM bands b
			JOIN events_bands eb ON b.id = eb.id_band
			WHERE eb.id_event = ?
		`, e.ID)
		if err == nil {
			var bands []Band
			for bandRows.Next() {
				var b Band
				if err := bandRows.Scan(&b.ID, &b.Name, &b.Slug); err == nil {

					bands = append(bands, b)
				}
			}
			bandRows.Close() // ✅ cerrar por cada evento
			e.Bands = bands
		}

		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (h *AuthHandler) GetEventByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var e Event
	var v Venue

	// check if id is numeric or is a slug
	var row *sql.Row
	var err error
	if _, errConv := strconv.Atoi(id); errConv == nil {
		// Es un número → buscar por ID
		row, err = h.DB.SelectRow(`

		SELECT

			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE e.id = ?
	`, id)
	} else {
		// No es número → buscar por slug
		row, err = h.DB.SelectRow(`
		SELECT
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE e.slug = ?
	`, id)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = row.Scan(&e.ID, &e.Title, &e.Tags, &e.Content, &e.Slug, &e.DateStart, &e.DateEnd,
		&v.ID, &v.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	e.Venue = &v

	// Bandas
	bandRows, err := h.DB.Select(`
		SELECT b.id, b.name, b.slug
		FROM bands b
		JOIN events_bands eb ON b.id = eb.id_band
		WHERE eb.id_event = ?
	`, e.ID)
	if err == nil {
		defer bandRows.Close()
		for bandRows.Next() {
			var b Band
			if err := bandRows.Scan(&b.ID, &b.Name, &b.Slug); err == nil {
				e.Bands = append(e.Bands, b)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(e)
}
func (h *AuthHandler) GetEventsByVenueID(w http.ResponseWriter, r *http.Request) {
	venueID := chi.URLParam(r, "id")
	query := `
		SELECT 
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE e.id_venue = ?
		ORDER BY e.date_start DESC
	`

	rows, err := h.DB.Select(query, venueID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var v Venue
		if err := rows.Scan(&e.ID, &e.Title, &e.Tags, &e.Content, &e.Slug, &e.DateStart, &e.DateEnd,
			&v.ID, &v.Name); err != nil {
			continue
		}
		e.Venue = &v

		// Bandas asociadas
		bandRows, _ := h.DB.Select(`
			SELECT b.id, b.name, b.slug
			FROM bands b
			JOIN events_bands eb ON b.id = eb.id_band
			WHERE eb.id_event = ?
		`, e.ID)
		defer bandRows.Close()
		for bandRows.Next() {
			var b Band

			if err := bandRows.Scan(&b.ID, &b.Name, &b.Slug); err == nil {
				e.Bands = append(e.Bands, b)
			}
		}

		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
func (h *AuthHandler) GetEventsByBandID(w http.ResponseWriter, r *http.Request) {
	bandID := chi.URLParam(r, "id")
	query := `
		SELECT 
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name
		FROM events e
		JOIN events_bands eb ON e.id = eb.id_event
		JOIN venues v ON e.id_venue = v.id
		WHERE eb.id_band = ?
		ORDER BY e.date_start DESC
	`

	rows, err := h.DB.Select(query, bandID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var v Venue
		if err := rows.Scan(&e.ID, &e.Title, &e.Tags, &e.Content, &e.Slug, &e.DateStart, &e.DateEnd,
			&v.ID, &v.Name); err != nil {
			continue
		}
		e.Venue = &v

		// Bandas asociadas
		bandRows, _ := h.DB.Select(`
			SELECT b.id, b.name, b.slug
			FROM bands b
			JOIN events_bands eb ON b.id = eb.id_band
			WHERE eb.id_event = ?
		`, e.ID)
		defer bandRows.Close()
		for bandRows.Next() {
			var b Band
			if err := bandRows.Scan(&b.ID, &b.Name, &b.Slug); err == nil {
				e.Bands = append(e.Bands, b)
			}
		}

		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (h *AuthHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	type EventInput struct {
		IDVenue   int    `json:"id_venue"`
		Title     string `json:"title"`
		Tags      string `json:"tags"`
		Content   string `json:"content"`
		Slug      string `json:"slug"`
		DateStart string `json:"date_start"`
		DateEnd   string `json:"date_end"`
		BandIDs   []int  `json:"band_ids"` // <-- Lista de bandas
	}

	var input EventInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Error al decodificar el cuerpo", http.StatusBadRequest)
		return
	}

	// Insertar el evento
	eventID, err := h.DB.Insert(false, `
		INSERT INTO events (id_venue, title, tags, content, slug, date_start, date_end)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		input.IDVenue, input.Title, input.Tags, input.Content, input.Slug, input.DateStart, input.DateEnd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insertar relaciones en events_bands
	for _, bandID := range input.BandIDs {
		_, err := h.DB.Insert(false, `
			INSERT INTO events_bands (id_band, id_event) VALUES (?, ?)`,
			bandID, eventID)
		if err != nil {
			// No detiene el proceso si falla una banda
			fmt.Printf("Error insertando banda %d: %v\n", bandID, err)
		}
	}

	// Devolver el evento creado
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         eventID,
		"title":      input.Title,
		"slug":       input.Slug,
		"tags":       input.Tags,
		"content":    input.Content,
		"date_start": input.DateStart,
		"date_end":   input.DateEnd,
		"id_venue":   input.IDVenue,
		"band_ids":   input.BandIDs,
	})
}

func (h *AuthHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	type EventInput struct {
		IDVenue   int    `json:"id_venue"`
		Title     string `json:"title"`
		Tags      string `json:"tags"`
		Content   string `json:"content"`
		Slug      string `json:"slug"`
		DateStart string `json:"date_start"`
		DateEnd   string `json:"date_end"`
		BandIDs   []int  `json:"band_ids"`
	}

	id := chi.URLParam(r, "id")
	var input EventInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Error al decodificar el cuerpo", http.StatusBadRequest)
		return
	}

	_, err := h.DB.Update(false, `
		UPDATE events SET id_venue=?, title=?, tags=?, content=?, slug=?, date_start=?, date_end=?
		WHERE id = ?`,
		input.IDVenue, input.Title, input.Tags, input.Content, input.Slug, input.DateStart, input.DateEnd, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Eliminar asociaciones anteriores
	_, _ = h.DB.Delete(false, "DELETE FROM events_bands WHERE id_event = ?", id)

	// Insertar nuevas asociaciones
	for _, bandID := range input.BandIDs {
		_, err := h.DB.Insert(false, `
			INSERT INTO events_bands (id_band, id_event) VALUES (?, ?)`,
			bandID, id)
		if err != nil {
			fmt.Printf("Error insertando banda %d: %v\n", bandID, err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (h *AuthHandler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Primero eliminar las relaciones en events_bands
	_, _ = h.DB.Delete(false, "DELETE FROM events_bands WHERE id_event = ?", id)

	// Luego eliminar el evento
	_, err := h.DB.Delete(false, "DELETE FROM events WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
