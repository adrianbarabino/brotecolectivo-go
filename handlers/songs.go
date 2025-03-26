package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-chi/chi/v5"
	"gopkg.in/ini.v1"
)

type Song struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	BandID   int    `json:"band_id"`
	GenreID  int    `json:"genre_id"`
	LyricsID int    `json:"lyrics_id"`
	Band     *Band  `json:"band,omitempty"`
	Genre    *Genre `json:"genre,omitempty"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Lyrics struct {
	ID     int    `json:"id"`
	Lyric  string `json:"lyric"`
	IDSong int    `json:"id_song"`
	Author string `json:"author"`
}

func (h *AuthHandler) GetLyricsByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, _ := h.DB.SelectRow("SELECT id, lyric, id_song, author FROM lyrics WHERE id = ?", id)
	var l Lyrics
	err := row.Scan(&l.ID, &l.Lyric, &l.IDSong, &l.Author)
	if err != nil {
		http.Error(w, "Letra no encontrada", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(l)
}

func (h *AuthHandler) GetSongs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Select(`
		SELECT s.id, s.title, s.slug, s.id_band, s.id_genre,
		       b.id, b.name, b.slug,
		       g.id, g.name, l.id
		FROM songs s
		LEFT JOIN bands b ON s.id_band = b.id
		LEFT JOIN genres g ON s.id_genre = g.id
		LEFT JOIN lyrics l ON s.id = l.id_song
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var bID sql.NullInt64
		var bName, bSlug sql.NullString
		var gID sql.NullInt64
		var gName sql.NullString
		var lID sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Slug, &s.BandID, &s.GenreID,
			&bID, &bName, &bSlug,
			&gID, &gName, &lID,
		)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if bID.Valid {
			s.Band = &Band{
				ID:   int(bID.Int64),
				Name: bName.String,
				Slug: bSlug.String,
			}
		}

		if gID.Valid {
			s.Genre = &Genre{
				ID:   int(gID.Int64),
				Name: gName.String,
			}
		}

		if lID.Valid {
			s.LyricsID = int(lID.Int64)
		}

		songs = append(songs, s)
	}

	json.NewEncoder(w).Encode(songs)
}

func (h *AuthHandler) GetSongByID(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")
	query := `
		SELECT s.id, s.title, s.slug, s.id_band, s.id_genre,
		       b.id, b.name, b.slug, 
		       g.id, g.name
		FROM songs s
		LEFT JOIN bands b ON s.id_band = b.id
		LEFT JOIN genres g ON s.id_genre = g.id
		WHERE `

	var row *sql.Row
	if isNumeric(idOrSlug) {
		query += "s.id = ?"
		row, _ = h.DB.SelectRow(query, idOrSlug)
	} else {
		query += "s.slug = ?"
		row, _ = h.DB.SelectRow(query, idOrSlug)
	}

	var s Song
	var b Band
	var g Genre
	err := row.Scan(
		&s.ID, &s.Title, &s.Slug, &s.BandID, &s.GenreID,
		&b.ID, &b.Name, &b.Slug,
		&g.ID, &g.Name,
	)
	if err != nil {
		http.Error(w, "Cancion no encontrada", http.StatusNotFound)
		return
	}

	s.Band = &b
	s.Genre = &g
	json.NewEncoder(w).Encode(s)
}

func (h *AuthHandler) CreateSong(w http.ResponseWriter, r *http.Request) {
	var s Song
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	lastID, err := h.DB.Insert(true, "INSERT INTO songs (title, slug, id_band, id_genre) VALUES (?, ?, ?, ?)",
		s.Title, s.Slug, s.BandID, s.GenreID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.ID = int(lastID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(s)
}

func (h *AuthHandler) UpdateSong(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")
	var s Song
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	query := "UPDATE songs SET title = ?, slug = ?, id_band = ?, id_genre = ? WHERE "
	args := []interface{}{s.Title, s.Slug, s.BandID, s.GenreID}
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
	w.WriteHeader(http.StatusOK)
}

func (h *AuthHandler) DeleteSong(w http.ResponseWriter, r *http.Request) {
	idOrSlug := chi.URLParam(r, "id")
	query := "DELETE FROM songs WHERE "
	if isNumeric(idOrSlug) {
		query += "id = ?"
	} else {
		query += "slug = ?"
	}
	_, err := h.DB.Delete(true, query, idOrSlug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) UploadSongAudio(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, handler, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "Error al leer el archivo", http.StatusBadRequest)
		return
	}
	defer file.Close()

	slug := chi.URLParam(r, "id")
	tempFile, err := os.CreateTemp("", "upload-*.input")
	if err != nil {
		http.Error(w, "No se pudo crear archivo temporal", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile.Name())
	io.Copy(tempFile, file)

	outputPath := fmt.Sprintf("converted-%s.mp3", slug)
	cmd := exec.Command("ffmpeg", "-i", tempFile.Name(), "-codec:a", "libmp3lame", "-qscale:a", "2", outputPath)
	err = cmd.Run()
	if err != nil {
		http.Error(w, "Error al convertir a mp3", http.StatusInternalServerError)
		return
	}
	defer os.Remove(outputPath)

	err = uploadToSpaces(outputPath, fmt.Sprintf("songs/%s.mp3", slug), handler.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, "Error al subir a Spaces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Audio subido con éxito"})
}

func uploadToSpaces(filePath, key, contentType string) error {
	config, err := ini.Load("data.conf")
	if err != nil {
		return fmt.Errorf("failed to load config file: %v", err)
	}

	accessKey := config.Section("spaces").Key("access_key").String()
	secretKey := config.Section("spaces").Key("secret_key").String()
	region := config.Section("spaces").Key("region").String()
	endpoint := config.Section("spaces").Key("endpoint").String()
	bucket := config.Section("spaces").Key("bucket").String()

	sess, _ := session.NewSession(&aws.Config{
		Region:           aws.String(region),
		Endpoint:         aws.String(endpoint),
		S3ForcePathStyle: aws.Bool(true),
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
	})

	svc := s3.New(sess)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
		ACL:         aws.String("public-read"),
	})

	return err
}
