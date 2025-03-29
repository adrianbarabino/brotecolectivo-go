package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Event representa un evento cultural en la plataforma Brote Colectivo.
// Contiene información sobre fechas, ubicación, artistas participantes y detalles del evento.
//
// @Schema
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
	VenueID   int    `json:"id_venue"` // <--- agregar esto
}

// GetEventsCount devuelve el número total de eventos en la base de datos.
//
// @Summary Obtener conteo de eventos
// @Description Devuelve el número total de eventos registrados en la plataforma
// @Tags eventos
// @Produce json
// @Success 200 {object} map[string]int "Conteo exitoso"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/count [get]
func (h *AuthHandler) GetEventsCount(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("q")
	idFilter := r.URL.Query().Get("id")
	titleFilter := r.URL.Query().Get("title")
	dateFilter := r.URL.Query().Get("date_start")

	query := `
		SELECT COUNT(*) 
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE 1=1
	`
	var queryParams []interface{}

	if search != "" {
		pattern := "%" + search + "%"
		query += ` AND (e.title LIKE ? OR e.tags LIKE ? OR e.content LIKE ? OR e.slug LIKE ?)`
		queryParams = append(queryParams, pattern, pattern, pattern, pattern)
	}
	if idFilter != "" {
		query += " AND e.id LIKE ?"
		queryParams = append(queryParams, "%"+idFilter+"%")
	}
	if titleFilter != "" {
		query += " AND e.title LIKE ?"
		queryParams = append(queryParams, "%"+titleFilter+"%")
	}
	if dateFilter != "" {
		query += " AND e.date_start LIKE ?"
		queryParams = append(queryParams, "%"+dateFilter+"%")
	}

	row, err := h.DB.SelectRow(query, queryParams...)
	if err != nil {
		http.Error(w, "Error al contar eventos", http.StatusInternalServerError)
		return
	}

	var count int
	if err := row.Scan(&count); err != nil {
		http.Error(w, "Error al leer el conteo", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"count": count})
}

// GetEventsDatatable devuelve los datos de eventos en formato para DataTables.
//
// @Summary Obtener eventos para DataTables
// @Description Devuelve los datos de eventos formateados para su uso con DataTables
// @Tags eventos
// @Produce json
// @Param draw query int false "Parámetro draw de DataTables"
// @Param start query int false "Índice de inicio para paginación"
// @Param length query int false "Número de registros a mostrar"
// @Param search[value] query string false "Término de búsqueda"
// @Param order[0][column] query int false "Índice de la columna para ordenar"
// @Param order[0][dir] query string false "Dirección de ordenamiento (asc/desc)"
// @Success 200 {object} DatatableResponse "Datos para DataTables"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/table [get]
func (h *AuthHandler) GetEventsDatatable(w http.ResponseWriter, r *http.Request) {
	offsetParam := r.URL.Query().Get("offset")
	limitParam := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("q")
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	idFilter := r.URL.Query().Get("id")
	titleFilter := r.URL.Query().Get("title")
	dateFilter := r.URL.Query().Get("date_start")

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
		pattern := "%" + search + "%"
		query += ` AND (e.title LIKE ? OR e.tags LIKE ? OR e.content LIKE ? OR e.slug LIKE ?)`
		queryParams = append(queryParams, pattern, pattern, pattern, pattern)
	}
	if idFilter != "" {
		query += " AND e.id LIKE ?"
		queryParams = append(queryParams, "%"+idFilter+"%")
	}
	if titleFilter != "" {
		query += " AND e.title LIKE ?"
		queryParams = append(queryParams, "%"+titleFilter+"%")
	}
	if dateFilter != "" {
		query += " AND e.date_start LIKE ?"
		queryParams = append(queryParams, "%"+dateFilter+"%")
	}

	if sortBy != "" {
		validSorts := map[string]bool{"id": true, "title": true, "date_start": true}
		if validSorts[sortBy] {
			if order != "desc" {
				order = "asc"
			}
			query += fmt.Sprintf(" ORDER BY e.%s %s", sortBy, strings.ToUpper(order))
		}
	}

	query += " LIMIT ? OFFSET ?"
	queryParams = append(queryParams, limit, offset)
	fmt.Println("QUERY:", query)
	fmt.Println("PARAMS:", queryParams)

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
			bandRows.Close()
			e.Bands = bands
		}

		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// GetEvents devuelve todos los eventos, con opciones de filtrado y paginación.
//
// @Summary Listar eventos
// @Description Obtiene una lista de eventos con opciones de filtrado y paginación
// @Tags eventos
// @Produce json
// @Param page query int false "Número de página para paginación"
// @Param limit query int false "Límite de registros por página"
// @Param search query string false "Término de búsqueda"
// @Param upcoming query bool false "Solo eventos futuros"
// @Param past query bool false "Solo eventos pasados"
// @Success 200 {array} Event "Lista de eventos"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events [get]
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
			bandRows.Close() // 
			e.Bands = bands
		}

		events = append(events, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// GetEventByID devuelve un evento específico por su ID.
//
// @Summary Obtener evento por ID
// @Description Obtiene los detalles completos de un evento específico por su ID
// @Tags eventos
// @Produce json
// @Param id path int true "ID del evento"
// @Success 200 {object} Event "Detalles del evento"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 404 {string} string "Evento no encontrado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/{id} [get]
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
			v.id, v.name, v.latlng, v.address, v.city
		FROM events e
		JOIN venues v ON e.id_venue = v.id
		WHERE e.id = ?
	`, id)
	} else {
		// No es número → buscar por slug
		row, err = h.DB.SelectRow(`
		SELECT
			e.id, e.title, e.tags, e.content, e.slug, e.date_start, e.date_end,
			v.id, v.name, v.latlng, v.address, v.city
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
		&v.ID, &v.Name, &v.LatLng, &v.Address, &v.City)
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

// GetEventsByVenueID devuelve todos los eventos asociados a un venue específico.
//
// @Summary Obtener eventos por venue
// @Description Obtiene todos los eventos que se realizan en un venue específico
// @Tags eventos
// @Produce json
// @Param id path int true "ID del venue"
// @Success 200 {array} Event "Lista de eventos del venue"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 404 {string} string "Venue no encontrado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/venue/{id} [get]
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

// GetEventsByBandID devuelve todos los eventos asociados a una banda específica.
//
// @Summary Obtener eventos por banda
// @Description Obtiene todos los eventos en los que participa una banda específica
// @Tags eventos
// @Produce json
// @Param id path int true "ID de la banda"
// @Success 200 {array} Event "Lista de eventos de la banda"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 404 {string} string "Banda no encontrada"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/band/{id} [get]
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

// UploadEventImage maneja la subida de imágenes para eventos.
//
// @Summary Subir imagen de evento
// @Description Sube una imagen para un evento y la almacena en DigitalOcean Spaces
// @Tags eventos
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Archivo de imagen a subir"
// @Param slug formData string true "Slug del evento"
// @Security BearerAuth
// @Success 200 {object} map[string]string "URL de la imagen subida"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/upload-image [post]
func (h *AuthHandler) UploadEventImage(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No se pudo leer el archivo", http.StatusBadRequest)
		return
	}
	defer file.Close()

	slug := r.FormValue("slug")
	if slug == "" {
		http.Error(w, "Slug faltante", http.StatusBadRequest)
		return
	}

	tempFile, err := os.CreateTemp("", "upload-*.jpg")
	if err != nil {
		http.Error(w, "No se pudo crear archivo temporal", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile.Name())
	io.Copy(tempFile, file)

	err = uploadToSpaces(tempFile.Name(), "events/"+slug+".jpg", handler.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, "Error al subir imagen: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Imagen subida con éxito"})
}

// CreateEvent crea un nuevo evento en la base de datos.
//
// @Summary Crear evento
// @Description Crea un nuevo evento con la información proporcionada
// @Tags eventos
// @Accept json
// @Produce json
// @Param event body Event true "Datos del evento a crear"
// @Security BearerAuth
// @Success 201 {object} Event "Evento creado exitosamente"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events [post]
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

// UpdateEvent actualiza un evento existente en la base de datos.
//
// @Summary Actualizar evento
// @Description Actualiza la información de un evento existente
// @Tags eventos
// @Accept json
// @Produce json
// @Param id path int true "ID del evento a actualizar"
// @Param event body Event true "Datos actualizados del evento"
// @Security BearerAuth
// @Success 200 {object} Event "Evento actualizado exitosamente"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 404 {string} string "Evento no encontrado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/{id} [put]
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

// DeleteEvent elimina un evento de la base de datos.
//
// @Summary Eliminar evento
// @Description Elimina un evento existente y sus relaciones con bandas
// @Tags eventos
// @Produce json
// @Param id path int true "ID del evento a eliminar"
// @Security BearerAuth
// @Success 200 {object} map[string]string "Mensaje de éxito"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 404 {string} string "Evento no encontrado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /events/{id} [delete]
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
