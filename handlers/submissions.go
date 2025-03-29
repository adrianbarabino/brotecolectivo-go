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
	"time"

	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-chi/chi/v5"
	"gopkg.in/ini.v1"
)

func (h *AuthHandler) DirectApprove(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	token := r.URL.Query().Get("token")

	cfg, err := ini.Load("data.conf")
	if err != nil {
		http.Error(w, "Config error", http.StatusInternalServerError)
		return
	}
	secret := cfg.Section("security").Key("approval_secret").String()
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}
	expectedToken := generateApprovalToken(id, secret)

	if token != expectedToken {
		http.Error(w, "Token inválido", http.StatusUnauthorized)
		return
	}

	// Get submission info before approval to determine type and details
	row, err := h.DB.SelectRow("SELECT type, data FROM submissions WHERE id = ? AND status = 'pending'", idStr)
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

	// Extract content details
	name, description, slug, _ := extractFieldsFromSubmission(Submission{
		ID:   id,
		Type: subType,
		Data: dataRaw,
	})

	// Prepare response writer to capture JSON response
	responseCapture := utils.NewResponseCapture(w)

	// Forzamos reviewer_id = 1 (o alguno por default)
	payload := struct {
		ReviewerID int `json:"reviewer_id"`
	}{ReviewerID: 1}

	body, _ := json.Marshal(payload)
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Call the original ApproveSubmission but with our response capture
	h.ApproveSubmission(responseCapture, r)

	// If there was an error, just return it as is
	if responseCapture.StatusCode >= 400 {
		return
	}

	// Parse the JSON response to get the ID
	var response map[string]interface{}
	if err := json.Unmarshal(responseCapture.Body.Bytes(), &response); err != nil {
		http.Error(w, "Error al procesar la respuesta", http.StatusInternalServerError)
		return
	}

	// Determine content type and ID
	var contentID int
	var contentType, viewURL string

	// Set default values to prevent MISSING errors
	contentType = "contenido"
	viewURL = "https://brotecolectivo.com"
	contentID = 0 // Default ID value

	switch subType {
	case "band":
		if id, ok := response["band_id"].(float64); ok {
			contentID = int(id)
			contentType = "artista"
			viewURL = fmt.Sprintf("https://brotecolectivo.com/artist/%s", slug)
		}
	case "event", "eventvenue":
		if id, ok := response["event_id"].(float64); ok {
			contentID = int(id)
			contentType = "evento"
			viewURL = fmt.Sprintf("https://brotecolectivo.com/event/%s", slug)
		}
	case "news":
		if id, ok := response["news_id"].(float64); ok {
			contentID = int(id)
			contentType = "noticia"
			viewURL = fmt.Sprintf("https://brotecolectivo.com/news/%s", slug)
		}
	case "video":
		if id, ok := response["video_id"].(float64); ok {
			contentID = int(id)
			contentType = "video"
			viewURL = fmt.Sprintf("https://brotecolectivo.com/videos")
		}
	case "song":
		if id, ok := response["song_id"].(float64); ok {
			contentID = int(id)
			contentType = "canción"
			viewURL = fmt.Sprintf("https://brotecolectivo.com/artist/%s", slug)
		}
	default:
		contentType = "contenido"
		viewURL = "https://brotecolectivo.com"
	}

	// Ensure we have safe values for all template variables
	if name == "" {
		name = "Sin nombre"
	}
	if description == "" {
		description = "Sin descripción disponible"
	}

	// Return a nice HTML page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Contenido Aprobado - Brote Colectivo</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Poppins:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: 'Poppins', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background-color: #f0f2f5;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            background-color: #ffffff;
            border-radius: 16px;
            padding: 30px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.08);
            width: 100%;
            max-width: 500px;
            text-align: center;
            position: relative;
            overflow: hidden;
        }
        .container::before {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
            height: 6px;
            background: linear-gradient(90deg, #4CAF50, #8BC34A);
        }
        .logo-container {
            margin-bottom: 25px;
            display: flex;
            justify-content: center;
        }
        .logo {
            max-width: 180px;
            height: auto;
        }
        h1 {
            color: #4CAF50;
            margin-bottom: 20px;
            font-weight: 700;
            font-size: 28px;
        }
        .success-icon {
            display: inline-block;
            width: 60px;
            height: 60px;
            background-color: #e8f5e9;
            border-radius: 50%;
            margin-bottom: 20px;
            position: relative;
        }
        .success-icon::before {
            content: '';
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -60%) rotate(45deg);
            width: 8px;
            height: 16px;
            border-bottom: 3px solid #4CAF50;
            border-right: 3px solid #4CAF50;
        }
        .content-type {
            display: inline-block;
            background-color: #e8f5e9;
            color: #4CAF50;
            padding: 5px 12px;
            border-radius: 20px;
            font-size: 14px;
            font-weight: 500;
            margin-bottom: 15px;
            text-transform: capitalize;
        }
        .content-name {
            font-size: 22px;
            font-weight: 600;
            margin-bottom: 25px;
            color: #333;
            word-break: break-word;
        }
        .content-info {
            margin: 25px 0;
            padding: 20px;
            background-color: #f9f9f9;
            border-radius: 12px;
            text-align: left;
        }
        .content-info p {
            margin-bottom: 10px;
            font-size: 15px;
        }
        .content-info p:last-child {
            margin-bottom: 0;
        }
        .content-info strong {
            color: #555;
            font-weight: 600;
        }
        .description {
            max-height: 120px;
            overflow-y: auto;
            text-align: left;
            padding: 10px;
            border-left: 3px solid #e0e0e0;
            margin-top: 10px;
            font-style: italic;
            color: #666;
            font-size: 14px;
        }
        .btn {
            display: inline-block;
            background: linear-gradient(90deg, #4CAF50, #8BC34A);
            color: white;
            padding: 14px 28px;
            text-decoration: none;
            border-radius: 30px;
            font-weight: 600;
            margin-top: 20px;
            transition: all 0.3s ease;
            box-shadow: 0 4px 15px rgba(76, 175, 80, 0.2);
            border: none;
            cursor: pointer;
        }
        .btn:hover {
            transform: translateY(-3px);
            box-shadow: 0 7px 20px rgba(76, 175, 80, 0.3);
        }
        .btn:active {
            transform: translateY(1px);
        }
        .footer {
            margin-top: 30px;
            font-size: 13px;
            color: #888;
        }
        .footer a {
            color: #4CAF50;
            text-decoration: none;
        }
        @media (max-width: 480px) {
            .container {
                padding: 25px 20px;
            }
            h1 {
                font-size: 24px;
            }
            .content-name {
                font-size: 20px;
            }
            .btn {
                padding: 12px 24px;
                font-size: 15px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="logo-container">
            <img src="https://brotecolectivo.com/img/logo.png" alt="Brote Colectivo" class="logo">
        </div>
        
        <div class="success-icon"></div>
        <h1>¡Contenido Aprobado!</h1>
        
        <span class="content-type">%s</span>
        <div class="content-name">%s</div>
        
        <div class="content-info">
            <p><strong>ID:</strong> %d</p>
            <p><strong>Descripción:</strong></p>
            <div class="description">%s</div>
        </div>
        
        <p>El contenido ha sido publicado exitosamente y ya está disponible en el sitio web.</p>
        
        <a href="%s" class="btn">Ver %s</a>
        
        <div class="footer">
            <p>Administración de <a href="https://brotecolectivo.com">Brote Colectivo</a></p>
        </div>
    </div>
</body>
</html>`, contentType, name, contentID, description, viewURL, contentType)

	w.Write([]byte(html))
}

func generateApprovalToken(submissionID int, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(strconv.Itoa(submissionID)))
	return hex.EncodeToString(h.Sum(nil))
}

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

func extractFieldsFromSubmission(sub Submission) (name, description, slug string, err error) {
	switch sub.Type {
	case "band":
		var data struct {
			Name string `json:"name"`
			Bio  string `json:"bio"`
			Slug string `json:"slug"`
		}
		err = json.Unmarshal(sub.Data, &data)
		return data.Name, data.Bio, data.Slug, err

	case "event":
		var data struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Slug    string `json:"slug"`
		}
		err = json.Unmarshal(sub.Data, &data)
		return data.Title, data.Content, data.Slug, err

	case "eventvenue":
		var data struct {
			Event struct {
				Title   string `json:"title"`
				Content string `json:"content"`
				Slug    string `json:"slug"`
			} `json:"event"`
		}
		err = json.Unmarshal(sub.Data, &data)
		return data.Event.Title, data.Event.Content, data.Event.Slug, err

	default:
		return "Desconocido", "Sin descripción", "unknown", nil
	}
}

func sendSubmissionWhatsApp(phone string, sub Submission, name, description, slug string, cfg *ini.File) error {
	// Inicio de debug
	fmt.Println("[WhatsApp Debug] Iniciando envío de WhatsApp")
	fmt.Printf("[WhatsApp Debug] Parámetros: phone=%s, submissionID=%d, type=%s, name=%s, slug=%s\n",
		phone, sub.ID, sub.Type, name, slug)

	imageURL := fmt.Sprintf("https://brotecolectivo.sfo3.cdn.digitaloceanspaces.com/pending/%s.jpg", slug)
	viewURL := fmt.Sprintf("%d", sub.ID)
	// Generar token con la secret
	secret := cfg.Section("security").Key("approval_secret").String()
	token := generateApprovalToken(sub.ID, secret)
	approveURLWithToken := fmt.Sprintf("%d?token=%s", sub.ID, token)

	// Debug de URLs
	fmt.Printf("[WhatsApp Debug] URLs generadas: imageURL=%s, approveURL=%s\n",
		imageURL, approveURLWithToken)

	message := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                phone,
		"type":              "template",
		"template": map[string]interface{}{
			"name": "nueva_colaboracion_brotecolectivo",
			"language": map[string]interface{}{
				"code": "es",
			},
			"components": []map[string]interface{}{
				{
					"type": "header",
					"parameters": []map[string]interface{}{
						{
							"type": "image",
							"image": map[string]string{
								"link": imageURL,
							},
						},
					},
				},
				{
					"type": "body",
					"parameters": []map[string]interface{}{
						{"type": "text", "text": sub.Type},
						{"type": "text", "text": fmt.Sprintf("%d", sub.UserID)},
						{"type": "text", "text": name},
						{"type": "text", "text": description},
					},
				},
				{
					"type":     "button",
					"sub_type": "url",
					"index":    0,
					"parameters": []map[string]interface{}{
						{"type": "text", "text": approveURLWithToken},
					},
				},
				{
					"type":     "button",
					"sub_type": "url",
					"index":    1,
					"parameters": []map[string]interface{}{
						{"type": "text", "text": viewURL},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		errMsg := fmt.Sprintf("[WhatsApp Debug] Error al convertir mensaje a JSON: %v", err)
		fmt.Println(errMsg)
		return fmt.Errorf(errMsg)
	}

	// Debug del payload JSON
	fmt.Printf("[WhatsApp Debug] Payload JSON: %s\n", string(jsonData))

	// Debug de configuración
	whatsappNumber := cfg.Section("keys").Key("whatsapp_number").String()
	whatsappToken := cfg.Section("keys").Key("whatsapp_token").String()
	fmt.Printf("[WhatsApp Debug] Configuración: whatsapp_number=%s, token_length=%d\n",
		whatsappNumber, len(whatsappToken))

	apiURL := fmt.Sprintf("https://graph.facebook.com/v17.0/%s/messages", whatsappNumber)
	fmt.Printf("[WhatsApp Debug] URL de API: %s\n", apiURL)

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		errMsg := fmt.Sprintf("[WhatsApp Debug] Error al crear request HTTP: %v", err)
		fmt.Println(errMsg)
		return fmt.Errorf(errMsg)
	}

	req.Header.Set("Authorization", "Bearer "+whatsappToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second, // Agregar timeout para evitar bloqueos indefinidos
	}

	fmt.Println("[WhatsApp Debug] Enviando request a la API de WhatsApp...")
	resp, err := client.Do(req)
	if err != nil {
		errMsg := fmt.Sprintf("[WhatsApp Debug] Error al enviar request: %v", err)
		fmt.Println(errMsg)
		return fmt.Errorf(errMsg)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[WhatsApp Debug] Respuesta de API (status=%d): %s\n", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errMsg := fmt.Sprintf("WhatsApp API error: %s", string(body))
		fmt.Println("[WhatsApp Debug] " + errMsg)
		return fmt.Errorf(errMsg)
	}

	fmt.Println("[WhatsApp Debug] Mensaje enviado exitosamente")
	return nil
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

		// Publicar en redes sociales
		imagePath := "bands/" + band.Slug + ".jpg"
		go func() {
			err := h.PublishToSocial("band", band, imagePath)
			idInt, _ := strconv.Atoi(id)
			if err != nil {
				h.LogSocialActivity(idInt, "band", false, err.Error())
			} else {
				h.LogSocialActivity(idInt, "band", true, "")
			}
		}()

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
			// dame un error mas detallado que el de arriba
			http.Error(w, "Error al crear venue: "+err.Error(), http.StatusInternalServerError)
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

		// Publicar en redes sociales
		imagePath := "events/" + event.Slug + ".jpg"
		go func() {
			err := h.PublishToSocial("eventvenue", raw, imagePath)
			idInt, _ := strconv.Atoi(id)
			if err != nil {
				h.LogSocialActivity(idInt, "eventvenue", false, err.Error())
			} else {
				h.LogSocialActivity(idInt, "eventvenue", true, "")
			}
		}()

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

		// Publicar en redes sociales
		imagePath := "events/" + event.Slug + ".jpg"
		go func() {
			err := h.PublishToSocial("event", event, imagePath)
			idInt, _ := strconv.Atoi(id)
			if err != nil {
				h.LogSocialActivity(idInt, "event", false, err.Error())
			} else {
				h.LogSocialActivity(idInt, "event", true, "")
			}
		}()

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

		// Publicar en redes sociales
		imagePath := "news/" + news.Slug + ".jpg"
		go func() {
			err := h.PublishToSocial("news", news, imagePath)
			idInt, _ := strconv.Atoi(id)
			if err != nil {
				h.LogSocialActivity(idInt, "news", false, err.Error())
			} else {
				h.LogSocialActivity(idInt, "news", true, "")
			}
		}()

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

	// asignar el RawMessage para poder usar extractFields
	s.Data = json.RawMessage(s.Data)

	// cargar configuración desde data.conf
	cfg, err := ini.Load("data.conf")
	if err == nil {
		// extraer campos y mandar WhatsApp
		name, description, slug, err := extractFieldsFromSubmission(s)
		if err == nil {
			// número del admin al que querés mandar el mensaje
			adminPhone := cfg.Section("keys").Key("admin_phone").String()
			if adminPhone != "" {
				fmt.Printf("[WhatsApp Debug] Iniciando envío asíncrono a %s para submission ID %d\n", adminPhone, s.ID)
				go func() {
					err := sendSubmissionWhatsApp(adminPhone, s, name, description, slug, cfg)
					if err != nil {
						fmt.Printf("[WhatsApp Debug] Error en envío asíncrono: %v\n", err)
					}
				}()
			} else {
				fmt.Println("[WhatsApp Debug] No se encontró número de admin en configuración")
			}
		} else {
			fmt.Printf("[WhatsApp Debug] Error al extraer campos: %v\n", err)
		}
	} else {
		fmt.Printf("[WhatsApp Debug] Error al cargar configuración: %v\n", err)
	}
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
