package handlers

import (
	"brotecolectivo/models"
	"brotecolectivo/utils"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-chi/chi/v5"
)

type Submission struct {
	ID        int             `json:"id"`
	UserID    int             `json:"user_id"`
	Type      string          `json:"type"` // ejemplo: "banda", "cancion", etc.
	Data      json.RawMessage `json:"data"`
	Status    string          `json:"status"` // pending, aprobado, rechazado
	Reviewer  *models.User    `json:"reviewer,omitempty"`
	Comment   string          `json:"comment,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func (h *AuthHandler) GetSubmissions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Select(`
		SELECT s.id, s.user_id, s.type, s.data, s.status, s.created_at, s.updated_at,
		       u.id, u.username, u.email
		FROM submissions s
		LEFT JOIN users u ON s.user_id = u.id
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

		err := rows.Scan(&s.ID, &s.UserID, &s.Type, &dataRaw, &s.Status, &s.CreatedAt, &s.UpdatedAt,
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
	var comment sql.NullString
	err = row.Scan(&s.ID, &s.UserID, &s.Type, &dataRaw, &s.Status, &comment, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		http.Error(w, "No encontrado", http.StatusNotFound)
		return
	}
	if comment.Valid {
		s.Comment = comment.String
	} else {
		s.Comment = ""
	}

	s.Data = dataRaw
	json.NewEncoder(w).Encode(s)
}
func moveImageInSpaces(oldKey, newKey string) error {
	accessKey, secretKey, region, endpoint, bucket, err := utils.LoadSpacesConfig()
	if err != nil {
		return err
	}

	sess, _ := session.NewSession(&aws.Config{
		Region:           aws.String(region),
		Endpoint:         aws.String(endpoint),
		S3ForcePathStyle: aws.Bool(true),
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
	})
	svc := s3.New(sess)

	// copiar
	_, err = svc.CopyObject(&s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: aws.String(bucket + "/" + oldKey),
		Key:        aws.String(newKey),
		ACL:        aws.String("public-read"),
	})
	if err != nil {
		return err
	}

	// eliminar la original
	_, err = svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(oldKey),
	})
	return err
}
func (h *AuthHandler) ApproveSubmission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		ReviewerID int `json:"reviewer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Error al decodificar datos", http.StatusBadRequest)
		return
	}

	row, err := h.DB.SelectRow("SELECT type, data FROM submissions WHERE id = ? AND status = 'pending'", id)
	if err != nil {
		http.Error(w, "No se pudo consultar la submission", http.StatusInternalServerError)
		return
	}

	var subType string
	var dataRaw []byte
	if err := row.Scan(&subType, &dataRaw); err != nil {
		http.Error(w, "Submission no encontrada o ya procesada", http.StatusNotFound)
		return
	}

	switch subType {
	case "band":
		var band Band
		if err := json.Unmarshal(dataRaw, &band); err != nil {
			http.Error(w, "Error al parsear datos", http.StatusInternalServerError)
			return
		}
		socialJSON, _ := json.Marshal(band.Social)
		newID, err := h.DB.Insert(false, `
			INSERT INTO bands (name, bio, slug, social)
			VALUES (?, ?, ?, ?)`, band.Name, band.Bio, band.Slug, string(socialJSON))
		if err != nil {
			http.Error(w, "Error al crear la banda: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = moveImageInSpaces("pending/"+band.Slug+".jpg", "bands/"+band.Slug+".jpg")
		_, _ = h.DB.Update(false, `UPDATE submissions SET status = 'approved', reviewed_by = ? WHERE id = ?`, payload.ReviewerID, id)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "band_id": newID})
	case "eventvenue":
		var raw struct {
			Venue map[string]interface{} `json:"venue"`
			Event map[string]interface{} `json:"event"`
		}
		if err := json.Unmarshal(dataRaw, &raw); err != nil {
			http.Error(w, "Error al parsear datos", http.StatusBadRequest)
			return
		}

		// Parsear Venue
		venue := Venue{
			Name:        raw.Venue["name"].(string),
			Slug:        raw.Venue["slug"].(string),
			Address:     raw.Venue["address"].(string),
			Description: raw.Venue["description"].(string),
			City:        raw.Venue["city"].(string),
			LatLng:      raw.Venue["latlng"].(string),
		}
		venueID, err := h.DB.Insert(false, `
			INSERT INTO venues (name, address, description, slug, latlng, city)
			VALUES (?, ?, ?, ?, ?, ?)`,
			venue.Name, venue.Address, venue.Description, venue.Slug, venue.LatLng, venue.City)
		if err != nil {
			http.Error(w, "Error al crear venue", http.StatusInternalServerError)
			return
		}

		// Parsear Event
		event := Event{
			Title:     raw.Event["title"].(string),
			Slug:      raw.Event["slug"].(string),
			Tags:      raw.Event["tags"].(string),
			Content:   raw.Event["content"].(string),
			DateStart: raw.Event["date_start"].(string),
			DateEnd:   raw.Event["date_end"].(string),
		}
		eventID, err := h.DB.Insert(false, `
			INSERT INTO events (id_venue, title, tags, content, slug, date_start, date_end)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			venueID, event.Title, event.Tags, event.Content, event.Slug, event.DateStart, event.DateEnd)
		if err != nil {
			http.Error(w, "Error al crear evento", http.StatusInternalServerError)
			return
		}

		// Asociar band_ids si existen
		if raw.Event["band_ids"] != nil {
			if ids, ok := raw.Event["band_ids"].([]interface{}); ok {
				for _, bid := range ids {
					if idFloat, ok := bid.(float64); ok {
						bandID := int(idFloat)
						_, _ = h.DB.Insert(false, `INSERT INTO events_bands (id_event, id_band) VALUES (?, ?)`, eventID, bandID)
					}
				}
			}
		}

		_ = moveImageInSpaces("pending/"+event.Slug+".jpg", "events/"+event.Slug+".jpg")
		_, _ = h.DB.Update(false, `UPDATE submissions SET status = 'approved', reviewed_by = ? WHERE id = ?`, payload.ReviewerID, id)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "event_id": eventID, "venue_id": venueID})

	case "event":
		var event Event
		if err := json.Unmarshal(dataRaw, &event); err != nil {
			http.Error(w, "Error al parsear datos", http.StatusBadRequest)
			return
		}
		eventID, err := h.DB.Insert(false, `
	INSERT INTO events (id_venue, title, tags, content, slug, date_start, date_end)
	VALUES (?, ?, ?, ?, ?, ?, ?)`,
			event.Venue.ID, event.Title, event.Tags, event.Content, event.Slug, event.DateStart, event.DateEnd)

		if err != nil {
			http.Error(w, "Error al crear evento", http.StatusInternalServerError)
			return
		}
		for _, band := range event.Bands {
			_, _ = h.DB.Insert(false, `INSERT INTO events_bands (id_event, id_band) VALUES (?, ?)`, eventID, band.ID)
		}
		_ = moveImageInSpaces("pending/"+event.Slug+".jpg", "events/"+event.Slug+".jpg")
		_, _ = h.DB.Update(false, `UPDATE submissions SET status = 'approved', reviewed_by = ? WHERE id = ?`, payload.ReviewerID, id)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "event_id": eventID})

	case "song":
		var song Song
		if err := json.Unmarshal(dataRaw, &song); err != nil {
			http.Error(w, "Error al parsear datos", http.StatusBadRequest)
			return
		}
		songID, err := h.DB.Insert(false, `INSERT INTO songs (title, slug, id_band, id_genre) VALUES (?, ?, ?, ?)`,
			song.Title, song.Slug, song.BandID, song.GenreID)
		if err != nil {
			http.Error(w, "Error al crear canción", http.StatusInternalServerError)
			return
		}
		_ = moveImageInSpaces("pending/"+song.Slug+".mp3", "songs/"+song.Slug+".mp3")
		_, _ = h.DB.Update(false, `UPDATE submissions SET status = 'approved', reviewed_by = ? WHERE id = ?`, payload.ReviewerID, id)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "song_id": songID})

	case "news":
		var news News
		if err := json.Unmarshal(dataRaw, &news); err != nil {
			http.Error(w, "Error al parsear datos", http.StatusBadRequest)
			return
		}
		newsID, err := h.DB.Insert(false, `INSERT INTO news (slug, title, content) VALUES (?, ?, ?)`,
			news.Slug, news.Title, news.Content)
		if err != nil {
			http.Error(w, "Error al crear noticia", http.StatusInternalServerError)
			return
		}
		for _, bandID := range news.BandIDs {
			_, _ = h.DB.Insert(false, `INSERT INTO news_bands (id_news, id_band) VALUES (?, ?)`, newsID, bandID)
		}
		_ = moveImageInSpaces("pending/"+news.Slug+".jpg", "news/"+news.Slug+".jpg")
		_, _ = h.DB.Update(false, `UPDATE submissions SET status = 'approved', reviewed_by = ? WHERE id = ?`, payload.ReviewerID, id)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "news_id": newsID})

	case "video":
		var video Video
		if err := json.Unmarshal(dataRaw, &video); err != nil {
			http.Error(w, "Error al parsear datos", http.StatusBadRequest)
			return
		}
		videoID, err := h.DB.Insert(false, `INSERT INTO videos (title, slug, id_youtube) VALUES (?, ?, ?)`,
			video.Title, video.Slug, video.YoutubeID)
		if err != nil {
			http.Error(w, "Error al crear video", http.StatusInternalServerError)
			return
		}
		for _, band := range video.Bands {
			_, _ = h.DB.Insert(false, `INSERT INTO videos_bands (id_video, id_band) VALUES (?, ?)`, videoID, band.ID)
		}
		_, _ = h.DB.Update(false, `UPDATE submissions SET status = 'approved', reviewed_by = ? WHERE id = ?`, payload.ReviewerID, id)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "video_id": videoID})

	default:
		http.Error(w, "Tipo de submission no soportado", http.StatusBadRequest)
	}
}

func (h *AuthHandler) UploadSubmissionImage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "No se pudo procesar la imagen", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No se pudo leer el archivo", http.StatusBadRequest)
		return
	}
	defer file.Close()

	slug := r.FormValue("slug")
	if slug == "" {
		http.Error(w, "Slug es requerido", http.StatusBadRequest)
		return
	}

	// decodificar (usando webp/png/jpg como ya tenés en bands.go)
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, file)
	if err != nil {
		http.Error(w, "Error leyendo archivo", http.StatusInternalServerError)
		return
	}

	contentType := http.DetectContentType(buf.Bytes())
	var src image.Image

	switch {
	case strings.Contains(contentType, "png"),
		strings.Contains(contentType, "gif"),
		strings.Contains(contentType, "jpeg"),
		strings.Contains(contentType, "jpg"):
		src, _, err = image.Decode(bytes.NewReader(buf.Bytes()))
	default:
		http.Error(w, "Formato no soportado (solo jpg, png, gif)", http.StatusBadRequest)
		return
	}

	// guardar como /pending/slug.jpg en Spaces
	tmpPath := fmt.Sprintf("tmp/%s.jpg", slug)
	if err := os.MkdirAll("tmp", os.ModePerm); err != nil {
		http.Error(w, "No se pudo crear carpeta temporal", http.StatusInternalServerError)
		return
	}
	out, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, "No se pudo crear archivo temporal", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	defer os.Remove(tmpPath)

	err = jpeg.Encode(out, src, &jpeg.Options{Quality: 85})
	if err != nil {
		http.Error(w, "No se pudo convertir a JPG", http.StatusInternalServerError)
		return
	}

	key := fmt.Sprintf("pending/%s.jpg", slug)
	err = uploadToSpaces(tmpPath, key, "image/jpeg")
	if err != nil {
		http.Error(w, "Error al subir a Spaces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"slug":   slug,
	})
}

func (h *AuthHandler) CreateSubmission(w http.ResponseWriter, r *http.Request) {
	var s Submission
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "Error al decodificar", http.StatusBadRequest)
		return
	}
	id, err := h.DB.Insert(false, `
		INSERT INTO submissions (user_id, type, data, status)
		VALUES (?, ?, ?, 'pending')`,
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
		Status     string          `json:"status"`
		Comment    string          `json:"comment"`
		ReviewerID int             `json:"reviewer_id"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Error al decodificar", http.StatusBadRequest)
		return
	}
	_, err := h.DB.Update(false, `
		UPDATE submissions SET status=?, comment=?, reviewed_by=?, data=? WHERE id=?`,
		payload.Status, payload.Comment, payload.ReviewerID, payload.Data, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
