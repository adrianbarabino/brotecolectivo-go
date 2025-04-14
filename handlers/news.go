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
	"time"

	"github.com/go-chi/chi/v5"
	"bytes"
)

// News representa un artículo de noticias en la plataforma Brote Colectivo.
// Contiene información sobre el contenido, autor, fecha y relaciones con bandas.
//
// @Schema
type News struct {
	ID      int    `json:"id"`
	Slug    string `json:"slug"`
	Date    string `json:"date"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Bands   []Band `json:"bands,omitempty"`
	BandIDs []int  `json:"band_ids,omitempty"` // Para crear/editar
}

// GetNewsCount devuelve el número total de noticias en la base de datos.
//
// @Summary Obtener conteo de noticias
// @Description Devuelve el número total de noticias registradas en la plataforma
// @Tags noticias
// @Produce json
// @Success 200 {object} map[string]int "Conteo exitoso"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news/count [get]
func (h *AuthHandler) GetNewsCount(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("q")
	idFilter := r.URL.Query().Get("id")
	titleFilter := r.URL.Query().Get("title")
	dateFilter := r.URL.Query().Get("date")

	query := `SELECT COUNT(*) FROM news WHERE 1=1`
	var queryParams []interface{}

	if search != "" {
		pattern := "%" + search + "%"
		query += ` AND (title LIKE ? OR content LIKE ? OR slug LIKE ?)`
		queryParams = append(queryParams, pattern, pattern, pattern)
	}
	if idFilter != "" {
		query += " AND id LIKE ?"
		queryParams = append(queryParams, "%"+idFilter+"%")
	}
	if titleFilter != "" {
		query += " AND title LIKE ?"
		queryParams = append(queryParams, "%"+titleFilter+"%")
	}
	if dateFilter != "" {
		query += " AND date LIKE ?"
		queryParams = append(queryParams, "%"+dateFilter+"%")
	}

	row, err := h.DB.SelectRow(query, queryParams...)
	if err != nil {
		http.Error(w, "Error al contar noticias", http.StatusInternalServerError)
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

// GetNewsDatatable devuelve los datos de noticias en formato para DataTables.
//
// @Summary Obtener noticias para DataTables
// @Description Devuelve los datos de noticias formateados para su uso con DataTables
// @Tags noticias
// @Produce json
// @Param draw query int false "Parámetro draw de DataTables"
// @Param start query int false "Índice de inicio para paginación"
// @Param length query int false "Número de registros a mostrar"
// @Param search[value] query string false "Término de búsqueda"
// @Param order[0][column] query int false "Índice de la columna para ordenar"
// @Param order[0][dir] query string false "Dirección de ordenamiento (asc/desc)"
// @Success 200 {object} DatatableResponse "Datos para DataTables"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news/table [get]
func (h *AuthHandler) GetNewsDatatable(w http.ResponseWriter, r *http.Request) {
	offsetParam := r.URL.Query().Get("offset")
	limitParam := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("q")
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	idFilter := r.URL.Query().Get("id")
	titleFilter := r.URL.Query().Get("title")
	dateFilter := r.URL.Query().Get("date")

	offset := 0
	limit := 10
	var err error

	if offsetParam != "" {
		offset, _ = strconv.Atoi(offsetParam)
	}
	if limitParam != "" {
		limit, _ = strconv.Atoi(limitParam)
	}

	query := `
		SELECT n.id, n.slug, n.date, n.title, n.content
		FROM news n
		WHERE 1=1
	`
	var queryParams []interface{}

	if search != "" {
		pattern := "%" + search + "%"
		query += ` AND (n.title LIKE ? OR n.content LIKE ? OR n.slug LIKE ?)`
		queryParams = append(queryParams, pattern, pattern, pattern)
	}
	if idFilter != "" {
		query += " AND n.id LIKE ?"
		queryParams = append(queryParams, "%"+idFilter+"%")
	}
	if titleFilter != "" {
		query += " AND n.title LIKE ?"
		queryParams = append(queryParams, "%"+titleFilter+"%")
	}
	if dateFilter != "" {
		query += " AND n.date LIKE ?"
		queryParams = append(queryParams, "%"+dateFilter+"%")
	}

	if sortBy != "" {
		validSorts := map[string]bool{"id": true, "title": true, "date": true}
		if validSorts[sortBy] {
			if order != "desc" {
				order = "asc"
			}
			query += fmt.Sprintf(" ORDER BY n.%s %s", sortBy, strings.ToUpper(order))
		}
	}

	query += " LIMIT ? OFFSET ?"
	queryParams = append(queryParams, limit, offset)

	rows, err := h.DB.Select(query, queryParams...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var newsList []News
	for rows.Next() {
		var n News
		if err := rows.Scan(&n.ID, &n.Slug, &n.Date, &n.Title, &n.Content); err != nil {
			continue
		}
		newsList = append(newsList, n)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newsList)
}

// UploadNewsImage maneja la subida de imágenes para noticias.
//
// @Summary Subir imagen de noticia
// @Description Sube una imagen para una noticia y la almacena en DigitalOcean Spaces
// @Tags noticias
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Archivo de imagen a subir"
// @Param slug formData string true "Slug de la noticia"
// @Security BearerAuth
// @Success 200 {object} map[string]string "URL de la imagen subida"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news/upload-image [post]
func (h *AuthHandler) UploadNewsImage(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB

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

	// Crear archivo temporal
	tempFile, err := os.CreateTemp("", "upload-*.jpg")
	if err != nil {
		http.Error(w, "No se pudo crear archivo temporal", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile.Name())
	io.Copy(tempFile, file)

	// Subir a Spaces o tu servicio de almacenamiento
	err = uploadToSpaces(tempFile.Name(), "news/"+slug+".jpg", handler.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, "Error al subir imagen: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Imagen subida con éxito",
	})
}

// GetNews devuelve todas las noticias, con opciones de filtrado y paginación.
//
// @Summary Listar noticias
// @Description Obtiene una lista de noticias con opciones de filtrado y paginación
// @Tags noticias
// @Produce json
// @Param page query int false "Número de página para paginación"
// @Param limit query int false "Límite de registros por página"
// @Param search query string false "Término de búsqueda"
// @Success 200 {array} News "Lista de noticias"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news [get]
func (h *AuthHandler) GetNews(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	limit := 10 // cantidad de noticias por página
	page := 1

	if pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err == nil && p > 0 {
			page = p
		}
	}
	offset := (page - 1) * limit

	rows, err := h.DB.Select(`
	SELECT n.id, n.slug, n.date, n.title, n.content, b.id, b.name, b.slug
	FROM (
		SELECT * FROM news
		ORDER BY id DESC
		LIMIT ? OFFSET ?
	) AS n
	LEFT JOIN news_bands nb ON n.id = nb.id_news
	LEFT JOIN bands b ON nb.id_band = b.id
`, limit, offset)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var allNews []News
	newsMap := make(map[int]*News)

	for rows.Next() {
		var nid int
		var slug, title, content, date string
		var bID sql.NullInt64
		var bName, bSlug sql.NullString

		err := rows.Scan(&nid, &slug, &date, &title, &content, &bID, &bName, &bSlug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if _, ok := newsMap[nid]; !ok {
			n := News{
				ID:      nid,
				Slug:    slug,
				Date:    date,
				Title:   title,
				Content: content,
				Bands:   []Band{},
			}
			newsMap[nid] = &n
			allNews = append(allNews, n) // guardás la referencia al orden
		}

		if bID.Valid {
			newsMap[nid].Bands = append(newsMap[nid].Bands, Band{
				ID:   int(bID.Int64),
				Name: bName.String,
				Slug: bSlug.String,
			})
		}
	}

	// como `allNews` tiene copias de los structs, y los actualizás en `newsMap`, tenés que reconstruir con referencias
	for i, n := range allNews {
		if updated, ok := newsMap[n.ID]; ok {
			allNews[i] = *updated
		}
	}

	json.NewEncoder(w).Encode(allNews)

}

// CreateNews crea una nueva noticia en la base de datos.
//
// @Summary Crear noticia
// @Description Crea una nueva noticia con la información proporcionada
// @Tags noticias
// @Accept json
// @Produce json
// @Param news body News true "Datos de la noticia a crear"
// @Security BearerAuth
// @Success 201 {object} News "Noticia creada exitosamente"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news [post]
func (h *AuthHandler) CreateNews(w http.ResponseWriter, r *http.Request) {
	var n News
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Convertir string "YYYY-MM-DD" a timestamp UNIX
	t, err := time.Parse("2006-01-02", n.Date)
	if err != nil {
		http.Error(w, "Formato de fecha inválido. Usá YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	timestamp := t.Unix()

	lastID, err := h.DB.Insert(true, `
		INSERT INTO news (slug, title, content, date) VALUES (?, ?, ?, ?)`,
		n.Slug, n.Title, n.Content, timestamp,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	n.ID = int(lastID)

	// Insertar en news_bands
	for _, bandID := range n.BandIDs {
		_, err := h.DB.Insert(false, `
			INSERT INTO news_bands (id_news, id_band) VALUES (?, ?)`,
			n.ID, bandID,
		)
		if err != nil {
			http.Error(w, "Error al vincular bandas", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(n)
}

// GetNewsByBandID devuelve todas las noticias asociadas a una banda específica.
//
// @Summary Obtener noticias por banda
// @Description Obtiene todas las noticias relacionadas con una banda específica
// @Tags noticias
// @Produce json
// @Param id path int true "ID de la banda"
// @Success 200 {array} News "Lista de noticias de la banda"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 404 {string} string "Banda no encontrada"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news/band/{id} [get]
func (h *AuthHandler) GetNewsByBandID(w http.ResponseWriter, r *http.Request) {
	bandID := chi.URLParam(r, "id")

	rows, err := h.DB.Select(`
		SELECT n.id, n.date, n.slug, n.title, n.content, b.id, b.name, b.slug
		FROM news n
		JOIN news_bands nb ON n.id = nb.id_news
		JOIN bands b ON nb.id_band = b.id
		WHERE nb.id_band = ?
		ORDER BY n.id
	`, bandID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var allNews []News
	newsMap := make(map[int]*News)

	for rows.Next() {
		var nid int
		var slug, title, content, date string
		var bID sql.NullInt64
		var bName, bSlug sql.NullString

		err := rows.Scan(&nid, &date, &slug, &title, &content, &bID, &bName, &bSlug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if _, ok := newsMap[nid]; !ok {
			n := News{
				ID:      nid,
				Slug:    slug,
				Date:    date,
				Title:   title,
				Content: content,
				Bands:   []Band{},
			}
			newsMap[nid] = &n
			allNews = append(allNews, n) // guardás la referencia al orden
		}

		if bID.Valid {
			newsMap[nid].Bands = append(newsMap[nid].Bands, Band{
				ID:   int(bID.Int64),
				Name: bName.String,
				Slug: bSlug.String,
			})
		}
	}

	// como `allNews` tiene copias de los structs, y los actualizás en `newsMap`, tenés que reconstruir con referencias
	for i, n := range allNews {
		if updated, ok := newsMap[n.ID]; ok {
			allNews[i] = *updated
		}
	}

	json.NewEncoder(w).Encode(allNews)

}

// GetNewsByID devuelve una noticia específica por su ID.
//
// @Summary Obtener noticia por ID
// @Description Obtiene los detalles completos de una noticia específica por su ID
// @Tags noticias
// @Produce json
// @Param id path int true "ID de la noticia"
// @Success 200 {object} News "Detalles de la noticia"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 404 {string} string "Noticia no encontrada"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news/{id} [get]
func (h *AuthHandler) GetNewsByID(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")

	var rows *sql.Rows
	var err error

	query := `
		SELECT n.id, n.slug, n.date, n.title, n.content, b.id, b.name, b.slug
		FROM news n
		LEFT JOIN news_bands nb ON n.id = nb.id_news
		LEFT JOIN bands b ON nb.id_band = b.id
		WHERE `
	if isNumeric(idOrSlug) {
		query += "n.id = ?"
		rows, err = h.DB.Select(query, idOrSlug)
	} else {
		query += "n.slug = ?"
		rows, err = h.DB.Select(query, idOrSlug)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var n *News
	for rows.Next() {
		var nid int
		var slug, title, content, date string
		var bID sql.NullInt64
		var bName, bSlug sql.NullString

		err := rows.Scan(&nid, &slug, &date, &title, &content, &bID, &bName, &bSlug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if n == nil {
			n = &News{
				ID:      nid,
				Slug:    slug,
				Date:    date,
				Title:   title,
				Content: content,
				Bands:   []Band{},
			}
		}

		if bID.Valid {
			b := Band{
				ID:   int(bID.Int64),
				Name: bName.String,
				Slug: bSlug.String,
			}
			n.Bands = append(n.Bands, b)
		}
	}

	if n == nil {
		http.Error(w, "Noticia no encontrada", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(n)
}

// UpdateNews actualiza una noticia existente en la base de datos.
//
// @Summary Actualizar noticia
// @Description Actualiza la información de una noticia existente
// @Tags noticias
// @Accept json
// @Produce json
// @Param id path int true "ID de la noticia a actualizar"
// @Param news body News true "Datos actualizados de la noticia"
// @Security BearerAuth
// @Success 200 {object} News "Noticia actualizada exitosamente"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 404 {string} string "Noticia no encontrada"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news/{id} [put]
func (h *AuthHandler) UpdateNews(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")
	var n News
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	query := "UPDATE news SET slug = ?, title = ?, content = ? WHERE "
	var args []interface{}
	args = append(args, n.Slug, n.Title, n.Content)

	if isNumeric(idOrSlug) {
		query += "id = ?"
		args = append(args, idOrSlug)
	} else {
		query += "slug = ?"
		args = append(args, idOrSlug)
	}

	_, err := h.DB.Update(true, query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Obtener el ID numérico para usarlo en la tabla intermedia
	var newsID int
	if isNumeric(idOrSlug) {
		newsID, _ = strconv.Atoi(idOrSlug)
	} else {
		row, _ := h.DB.SelectRow("SELECT id FROM news WHERE slug = ?", idOrSlug)
		_ = row.Scan(&newsID)
	}

	// Eliminar bandas anteriores
	_, err = h.DB.Delete(false, "DELETE FROM news_bands WHERE id_news = ?", newsID)
	if err != nil {
		http.Error(w, "Error al limpiar relaciones previas", http.StatusInternalServerError)
		return
	}

	// Insertar nuevas
	for _, bandID := range n.BandIDs {
		_, err := h.DB.Insert(false, `
			INSERT INTO news_bands (id_news, id_band) VALUES (?, ?)`,
			newsID, bandID,
		)
		if err != nil {
			http.Error(w, "Error al vincular bandas", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteNews elimina una noticia de la base de datos.
//
// @Summary Eliminar noticia
// @Description Elimina una noticia existente
// @Tags noticias
// @Produce json
// @Param id path int true "ID de la noticia a eliminar"
// @Security BearerAuth
// @Success 200 {object} map[string]string "Mensaje de éxito"
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 404 {string} string "Noticia no encontrada"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news/{id} [delete]
func (h *AuthHandler) DeleteNews(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")

	var id string
	if isNumeric(idOrSlug) {
		id = idOrSlug
	} else {
		row, err := h.DB.SelectRow("SELECT id FROM news WHERE slug = ?", idOrSlug)
		if err != nil {
			http.Error(w, "Error al buscar noticia", http.StatusInternalServerError)
			return
		}
		if err := row.Scan(&id); err != nil {
			http.Error(w, "Noticia no encontrada", http.StatusNotFound)
			return
		}
	}

	_, err := h.DB.Delete(false, "DELETE FROM news_bands WHERE id_news = ?", id)
	if err != nil {
		http.Error(w, "Error al eliminar relaciones", http.StatusInternalServerError)
		return
	}

	_, err = h.DB.Delete(true, "DELETE FROM news WHERE id = ?", id)
	if err != nil {
		http.Error(w, "Error al eliminar noticia", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GenerateNewsContentRequest representa la solicitud para generar contenido de noticia
type GenerateNewsContentRequest struct {
	Title        string `json:"title"`
	CustomPrompt string `json:"custom_prompt"`
}

// GenerateNewsContentResponse representa la respuesta del contenido generado
type GenerateNewsContentResponse struct {
	Content string `json:"content"`
}

// GenerateNewsContent genera contenido para una noticia usando OpenAI
// @Summary Generar contenido de noticia con IA
// @Description Genera contenido para una noticia usando OpenAI basado en el título y prompt personalizado
// @Tags news
// @Accept json
// @Produce json
// @Param request body GenerateNewsContentRequest true "Datos para generar el contenido"
// @Security BearerAuth
// @Success 200 {object} GenerateNewsContentResponse
// @Failure 400 {string} string "Error en la solicitud"
// @Failure 401 {string} string "No autorizado"
// @Failure 500 {string} string "Error interno del servidor"
// @Router /news/generate-content [post]
func (h *AuthHandler) GenerateNewsContent(w http.ResponseWriter, r *http.Request) {
	var req GenerateNewsContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	prompt := fmt.Sprintf(`Genera una noticia sobre: %s

    %s

    Instrucciones:
    1. Escribe en español de Argentina
    2. Usa un tono periodístico y profesional
    3. No inventes datos que no se hayan proporcionado
    4. Estructura: Comienza con un párrafo introductorio, luego desarrolla los detalles, y finaliza con un cierre
    5. Si se proporciona información adicional, agrégala solo si es factual y relevante
    6. Usa formato HTML para estructurar el contenido
    7. Separa bien los párrafos usando etiquetas <p>
    8. Evita usar "Ven", "eres" y otras expresiones españolas
    9. Siempre en tercera persona
    10. Ideal entre 3-4 párrafos`,
        req.Title,
        req.CustomPrompt)

	openaiURL := "https://api.openai.com/v1/chat/completions"
	requestBody := map[string]interface{}{
		"model": "gpt-3.5-turbo",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.5,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		http.Error(w, "Error al preparar la solicitud", http.StatusInternalServerError)
		return
	}

	httpReq, err := http.NewRequest("POST", openaiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		http.Error(w, "Error al crear la solicitud", http.StatusInternalServerError)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+getOpenAIKey())

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, "Error al realizar la solicitud a OpenAI", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var openaiResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&openaiResponse); err != nil {
		http.Error(w, "Error al procesar la respuesta de OpenAI", http.StatusInternalServerError)
		return
	}

	if len(openaiResponse.Choices) == 0 {
		http.Error(w, "No se recibió contenido de OpenAI", http.StatusInternalServerError)
		return
	}

	response := GenerateNewsContentResponse{
		Content: openaiResponse.Choices[0].Message.Content,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Ayuda para chequear si es numérico
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
