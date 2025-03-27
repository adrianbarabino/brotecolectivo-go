package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Video struct {
	ID        int     `json:"id"`
	Title     string  `json:"title"`
	Slug      string  `json:"slug"`
	YoutubeID string  `json:"youtube_id"`
	Bands     []*Band `json:"bands,omitempty"`
}

func (h *AuthHandler) GetVideos(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Select(`
		SELECT v.id, v.title, v.slug, v.id_youtube, b.id, b.name, b.slug
		FROM videos v
		LEFT JOIN videos_bands vb ON v.id = vb.id_video
		LEFT JOIN bands b ON vb.id_band = b.id
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	videosMap := map[int]*Video{}
	var videos []Video

	for rows.Next() {
		var id int
		var title, slug, youtubeID string
		var bID sql.NullInt64
		var bName, bSlug sql.NullString

		err := rows.Scan(&id, &title, &slug, &youtubeID, &bID, &bName, &bSlug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		video, exists := videosMap[id]
		if !exists {
			video = &Video{
				ID:        id,
				Title:     title,
				Slug:      slug,
				YoutubeID: youtubeID,
				Bands:     []*Band{},
			}
			videosMap[id] = video
		}

		if bID.Valid {
			video.Bands = append(video.Bands, &Band{
				ID:   int(bID.Int64),
				Name: bName.String,
				Slug: bSlug.String,
			})
		}
	}
	for _, v := range videosMap {
		videos = append(videos, *v)
	}

	json.NewEncoder(w).Encode(videos)
}

func (h *AuthHandler) GetVideoByID(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")

	query := `
		SELECT v.id, v.title, v.slug, v.id_youtube, b.id, b.name, b.slug
		FROM videos v
		LEFT JOIN videos_bands vb ON v.id = vb.id_video
		LEFT JOIN bands b ON vb.id_band = b.id
		WHERE `

	var rows *sql.Rows
	var err error
	if isNumeric(idOrSlug) {
		query += "v.id = ?"
		rows, err = h.DB.Select(query, idOrSlug)
	} else {
		query += "v.slug = ?"
		rows, err = h.DB.Select(query, idOrSlug)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var video *Video

	for rows.Next() {
		var id int
		var title, slug, youtubeID string
		var bID sql.NullInt64
		var bName, bSlug sql.NullString

		err := rows.Scan(&id, &title, &slug, &youtubeID, &bID, &bName, &bSlug)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if video == nil {
			video = &Video{
				ID:        id,
				Title:     title,
				Slug:      slug,
				YoutubeID: youtubeID,
				Bands:     []*Band{},
			}
		}

		if bID.Valid {
			video.Bands = append(video.Bands, &Band{
				ID:   int(bID.Int64),
				Name: bName.String,
				Slug: bSlug.String,
			})
		}
	}

	if video == nil {
		http.Error(w, "Video no encontrado", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(video)
}

func (h *AuthHandler) GetVideosByBandID(w http.ResponseWriter, r *http.Request) {
	bandID := chi.URLParam(r, "id")
	query := `
		SELECT v.id, v.title, v.slug, v.id_youtube
		FROM videos v
		JOIN videos_bands vb ON v.id = vb.id_video
		WHERE vb.id_band = ?
	`
	rows, err := h.DB.Select(query, bandID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Slug, &v.YoutubeID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		videos = append(videos, v)
	}

	json.NewEncoder(w).Encode(videos)
}

func (h *AuthHandler) CreateVideo(w http.ResponseWriter, r *http.Request) {
	var v Video
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	lastID, err := h.DB.Insert(true, "INSERT INTO videos (title, slug, id_youtube) VALUES (?, ?, ?)",
		v.Title, v.Slug, v.YoutubeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v.ID = int(lastID)

	// Insertar en videos_bands
	for _, band := range v.Bands {
		if band.ID > 0 {
			_, err = h.DB.Insert(true, "INSERT INTO videos_bands (id_video, id_band) VALUES (?, ?)", v.ID, band.ID)
			if err != nil {
				http.Error(w, "Video creado pero falló el vínculo con alguna banda", http.StatusInternalServerError)
				return
			}
		}
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(v)
}

func (h *AuthHandler) UpdateVideo(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")
	var v Video
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	query := "UPDATE videos SET title = ?, slug = ?, id_youtube = ? WHERE "
	args := []interface{}{v.Title, v.Slug, v.YoutubeID}

	if isNumeric(idOrSlug) {
		query += "id = ?"
	} else {
		query += "slug = ?"
	}
	args = append(args, idOrSlug)

	_, err := h.DB.Update(true, query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// También podés actualizar videos_bands si necesitás
	w.WriteHeader(http.StatusOK)
}

func (h *AuthHandler) DeleteVideo(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")
	query := "DELETE FROM videos WHERE "
	var arg interface{} = idOrSlug
	if isNumeric(idOrSlug) {
		query += "id = ?"
	} else {
		query += "slug = ?"
	}
	_, err := h.DB.Delete(true, query, arg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
