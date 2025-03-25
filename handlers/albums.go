package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Album struct {
	ID         int    `json:"id"`
	IDFacebook string `json:"id_facebook"`
	Title      string `json:"title"`
	Slug       string `json:"slug"`
}

func (h *AuthHandler) GetAlbums(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Select("SELECT id, id_Facebook, title, slug FROM albums")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var albums []Album
	for rows.Next() {
		var b Album
		rows.Scan(&b.ID, &b.IDFacebook, &b.Title, &b.Slug)
		albums = append(albums, b)
	}
	json.NewEncoder(w).Encode(albums)
}

func (h *AuthHandler) GetAlbumByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var b Album
	row, err := h.DB.SelectRow("SELECT id, id_Facebook, title, slug FROM albums WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = row.Scan(&b.ID, &b.IDFacebook, &b.Title, &b.Slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(b)
}

func (h *AuthHandler) CreateAlbum(w http.ResponseWriter, r *http.Request) {
	var b Album
	json.NewDecoder(r.Body).Decode(&b)
	id, err := h.DB.Insert(false, "INSERT INTO albums (id_Facebook, title, slug) VALUES (?, ?, ?, ?)", b.IDFacebook, b.Title, b.Slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	b.ID = int(id)
	json.NewEncoder(w).Encode(b)
}

func (h *AuthHandler) UpdateAlbum(w http.ResponseWriter, r *http.Request) {
	var b Album
	id := chi.URLParam(r, "id")
	_, err := h.DB.Update(false, "UPDATE albums SET name=?, bio=?, slug=?, social=? WHERE id=?", b.IDFacebook, b.Title, b.Slug, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *AuthHandler) DeleteAlbum(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := h.DB.Delete(false, "DELETE FROM albums WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
