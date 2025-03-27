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
)

type News struct {
	ID      int    `json:"id"`
	Slug    string `json:"slug"`
	Date    string `json:"date"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Bands   []Band `json:"bands,omitempty"`
	BandIDs []int  `json:"band_ids,omitempty"` // Para crear/editar
}

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

// get news by band id (not slug)
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

func (h *AuthHandler) DeleteNews(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")

	var newsID int
	if isNumeric(idOrSlug) {
		newsID, _ = strconv.Atoi(idOrSlug)
	} else {
		row, _ := h.DB.SelectRow("SELECT id FROM news WHERE slug = ?", idOrSlug)
		if err := row.Scan(&newsID); err != nil {
			http.Error(w, "Noticia no encontrada", http.StatusNotFound)
			return
		}
	}

	// Borrar relaciones en news_bands
	_, err := h.DB.Delete(false, "DELETE FROM news_bands WHERE id_news = ?", newsID)
	if err != nil {
		http.Error(w, "Error al eliminar relaciones", http.StatusInternalServerError)
		return
	}

	// Borrar la noticia
	_, err = h.DB.Delete(true, "DELETE FROM news WHERE id = ?", newsID)
	if err != nil {
		http.Error(w, "Error al eliminar noticia", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
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

// Ayuda para chequear si es numérico
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
