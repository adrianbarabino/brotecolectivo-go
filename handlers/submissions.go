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
	// Diagnóstico de tabla artist_links
	h.DB.CheckArtistLinksTable()

	idStr := chi.URLParam(r, "id")
	token := r.URL.Query().Get("token")

	// Agregar logs para depuración
	fmt.Println("DirectApprove - ID recibido:", idStr)
	fmt.Println("DirectApprove - Token recibido:", token)

	cfg, err := ini.Load("data.conf")
	if err != nil {
		fmt.Println("DirectApprove - Error al cargar config:", err)
		http.Error(w, "Config error", http.StatusInternalServerError)
		return
	}

	// Intentar obtener el secreto de la sección security primero
	secret := cfg.Section("security").Key("approval_secret").String()
	if secret == "" {
		// Intentar obtener de la sección keys como fallback
		secret = cfg.Section("keys").Key("approval_secret").String()
		fmt.Println("DirectApprove - Secret obtenido de la sección 'keys'")
	} else {
		fmt.Println("DirectApprove - Secret obtenido de la sección 'security'")
	}

	fmt.Println("DirectApprove - Secret cargado (longitud):", len(secret))

	id, err := strconv.Atoi(idStr)
	if err != nil {
		fmt.Println("DirectApprove - Error al convertir ID a entero:", err)
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}
	expectedToken := generateApprovalToken(id, secret)
	fmt.Println("DirectApprove - Token esperado:", expectedToken)

	if token != expectedToken {
		fmt.Println("DirectApprove - Token inválido. Recibido:", token, "Esperado:", expectedToken)
		http.Error(w, "Token inválido", http.StatusUnauthorized)
		return
	}

	fmt.Println("DirectApprove - Token válido, continuando con la aprobación")

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
	name, description, slug, err := extractFieldsFromSubmission(Submission{
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
	var viewURL string

	// Set default values to prevent MISSING errors
	contentID = 0 // Default ID value
	viewURL = "https://brotecolectivo.com"

	switch subType {
	case "band":
		if id, ok := response["band_id"].(float64); ok {
			contentID = int(id)
			viewURL = fmt.Sprintf("https://brotecolectivo.com/artista/%s", slug)
		}
	case "event", "eventvenue":
		if id, ok := response["event_id"].(float64); ok {
			contentID = int(id)
			viewURL = fmt.Sprintf("https://brotecolectivo.com/agenda-cultural/%s", slug)
		}
	case "news":
		if id, ok := response["news_id"].(float64); ok {
			contentID = int(id)
			viewURL = fmt.Sprintf("https://brotecolectivo.com/noticias/%s", slug)
		}
	case "video":
		if id, ok := response["video_id"].(float64); ok {
			contentID = int(id)
			viewURL = fmt.Sprintf("https://brotecolectivo.com/videos")
		}
	case "song":
		if id, ok := response["song_id"].(float64); ok {
			contentID = int(id)
			viewURL = fmt.Sprintf("https://brotecolectivo.com/artist/%s", slug)
		}
	case "artist_link":
		viewURL = "https://brotecolectivo.com"
	default:
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
	// Agregar encabezados CSP para permitir recursos externos
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' https://brotecolectivo.com")

	// Crear la plantilla HTML con los valores directamente insertados en lugar de usar fmt.Sprintf
	html := `<!DOCTYPE html>
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
            background-color: #333; /* Añadir fondo oscuro para el logo blanco */
            padding: 10px;
            border-radius: 8px;
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
        
        <span class="content-type">` + subType + `</span>
        <div class="content-name">` + name + `</div>
        
        <div class="content-info">
            <p><strong>ID:</strong> ` + strconv.Itoa(contentID) + `</p>
            <p><strong>Descripción:</strong></p>
            <div class="description">` + description + `</div>
        </div>
        
        <p>El contenido ha sido publicado exitosamente y ya está disponible en el sitio web.</p>
        
        <a href="` + viewURL + `" class="btn">Ver contenido</a>
        
        <div class="footer">
            <p>Administración de <a href="https://brotecolectivo.com">Brote Colectivo</a></p>
        </div>
    </div>
</body>
</html>`

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
	Type      string          `json:"type"` // ejemplo: "banda", "cancion", "event", "news", "video", "artist_link"
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
		var raw struct {
			Venue map[string]interface{} `json:"venue"`
			Event map[string]interface{} `json:"event"`
		}
		err = json.Unmarshal(sub.Data, &raw)
		return raw.Event["title"].(string), raw.Event["content"].(string), raw.Event["slug"].(string), err

	case "artist_link":
		var submissionData struct {
			Data struct {
				ArtistID int    `json:"artist_id"`
				Name     string `json:"name"`
				Slug     string `json:"slug"`
				Rol      string `json:"rol"`
				WhatsApp string `json:"whatsapp"`
			} `json:"data"`
		}

		if err = json.Unmarshal(sub.Data, &submissionData); err != nil {
			// Intentar con la estructura antigua
			var data struct {
				ArtistID int    `json:"artist_id"`
				Name     string `json:"name"`
				Slug     string `json:"slug"`
				Rol      string `json:"rol"`
				WhatsApp string `json:"whatsapp"`
			}
			err = json.Unmarshal(sub.Data, &data)
			return data.Name, "", data.Slug, err
		}

		return submissionData.Data.Name, "", submissionData.Data.Slug, err

	default:
		return "Desconocido", "Sin descripción", "unknown", nil
	}
}

func sendSubmissionWhatsApp(phone string, sub Submission, name, description, slug string, cfg *ini.File) error {
	// Obtener la URL base para el panel de administración
	// Generar token de aprobación
	secret := cfg.Section("security").Key("approval_secret").String()
	if secret == "" {
		// Intentar obtener de la sección keys como fallback
		secret = cfg.Section("keys").Key("approval_secret").String()
	}

	token := generateApprovalToken(sub.ID, secret)

	// URL para aprobar directamente
	approveURL := fmt.Sprintf("%d?token=%s", sub.ID, token)

	// Debug de la URL de aprobación
	fmt.Printf("[WhatsApp Debug] URL de aprobación generada: %s\n", approveURL)
	fmt.Printf("[WhatsApp Debug] Token generado: %s\n", token)
	fmt.Printf("[WhatsApp Debug] Secret usado (longitud): %d\n", len(secret))

	// Determinar el tipo de contenido y URL de visualización
	var viewURL string
	switch sub.Type {
	case "band":
		viewURL = fmt.Sprintf("https://brotecolectivo.com/artista/%s", slug)
	case "event":
		viewURL = fmt.Sprintf("https://brotecolectivo.com/agenda-cultural/%s", slug)
	case "eventvenue":
		viewURL = fmt.Sprintf("https://brotecolectivo.com/agenda-cultural/%s", slug)
	case "news":
		viewURL = fmt.Sprintf("https://brotecolectivo.com/noticias/%s", slug)
	case "song":
		viewURL = fmt.Sprintf("https://brotecolectivo.com/artist/%s", slug)
	case "artist_link":
		viewURL = fmt.Sprintf("https://brotecolectivo.com/artist/%s", slug)
	default:
		viewURL = "https://brotecolectivo.com"
	}

	// Inicio de debug
	fmt.Println("[WhatsApp Debug] Iniciando envío de WhatsApp")
	fmt.Printf("[WhatsApp Debug] Parámetros: phone=%s, submissionID=%d, type=%s, name=%s, slug=%s\n",
		phone, sub.ID, sub.Type, name, slug)

	// Debug de configuración
	whatsappNumber := cfg.Section("keys").Key("whatsapp_number").String()
	whatsappToken := cfg.Section("keys").Key("whatsapp_token").String()
	fmt.Printf("[WhatsApp Debug] Configuración: whatsapp_number=%s, token_length=%d\n",
		whatsappNumber, len(whatsappToken))

	// Asegurarse de que los valores no estén vacíos
	if name == "" {
		name = "Sin nombre"
	}
	if slug == "" {
		slug = "sin-slug"
	}
	if description == "" {
		description = "Sin descripción"
	}

	// Asegurarse de que el tipo no esté vacío
	submissionType := sub.Type
	if submissionType == "" {
		submissionType = "desconocido"
	}

	// Asegurarse de que el ID de usuario sea una cadena no vacía
	userIDStr := fmt.Sprintf("%d", sub.UserID)
	if userIDStr == "0" {
		userIDStr = "1" // Valor predeterminado para evitar errores
	}

	// Personalizar la descripción según el tipo de submission
	customDescription := description
	if sub.Type == "event" || sub.Type == "eventvenue" {
		customDescription = fmt.Sprintf("EVENTO: %s (ID: %d)", description, sub.ID)
	} else {
		customDescription = fmt.Sprintf("%s (ID: %d)", description, sub.ID)
	}

	imageURL := fmt.Sprintf("https://brotecolectivo.sfo3.cdn.digitaloceanspaces.com/pending/%s.jpg", slug)
	// Debug de URLs
	fmt.Printf("[WhatsApp Debug] URLs generadas: imageURL=%s, approveURL=%s\n",
		imageURL, approveURL)

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
						{"type": "text", "text": submissionType},
						{"type": "text", "text": userIDStr},
						{"type": "text", "text": name},
						{"type": "text", "text": customDescription},
					},
				},
				{
					"type":     "button",
					"sub_type": "url",
					"index":    0,
					"parameters": []map[string]interface{}{
						{"type": "text", "text": approveURL},
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
	row, err := h.DB.SelectRow("SELECT id, user_id, type, data, status, comment, created_at, updated_at FROM submissions WHERE id = ?", id)
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
	// Diagnóstico de tabla artist_links
	h.DB.CheckArtistLinksTable()

	// Extraer ID de la submission
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "ID de submission requerido", http.StatusBadRequest)
		return
	}

	// Decodificar el cuerpo de la solicitud
	var payload struct {
		ReviewerID int `json:"reviewer_id"`
	}

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		// Si hay un error al decodificar, usamos un valor predeterminado
		fmt.Println("Error al decodificar payload:", err)
		payload.ReviewerID = 1 // Valor predeterminado si no se proporciona
	}

	// Asegurarnos de que el ID del revisor sea válido
	if payload.ReviewerID <= 0 {
		payload.ReviewerID = 1
	}

	fmt.Println("Aprobando submission ID:", id, "Reviewer ID:", payload.ReviewerID)

	// Obtener datos de la submission
	row, err := h.DB.SelectRow("SELECT type, data, status FROM submissions WHERE id = ?", id)
	if err != nil {
		fmt.Println("Error al obtener submission:", err)
		http.Error(w, "Error al obtener submission: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var submissionType, status string
	var dataRaw json.RawMessage
	if err := row.Scan(&submissionType, &dataRaw, &status); err != nil {
		fmt.Println("Error al leer datos de submission:", err)
		http.Error(w, "Error al leer datos de submission: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println("Tipo de submission:", submissionType)
	fmt.Println("Estado actual:", status)
	fmt.Println("Datos raw:", string(dataRaw))

	// Obtener datos completos de la submission para depuración
	fmt.Println("Obteniendo datos completos de la submission ID:", id)
	var submissionUserID int
	fullSubmissionRow, err := h.DB.SelectRow("SELECT id, type, data, status, user_id, comment FROM submissions WHERE id = ?", id)
	if err != nil {
		fmt.Println("Error al obtener datos completos de la submission:", err)
	} else {
		var (
			submissionID      int
			submissionType    string
			submissionData    []byte
			submissionStatus  string
			submissionComment sql.NullString
		)
		if err := fullSubmissionRow.Scan(&submissionID, &submissionType, &submissionData, &submissionStatus, &submissionUserID, &submissionComment); err != nil {
			fmt.Println("Error al escanear datos completos de la submission:", err)
		} else {
			fmt.Println("Datos completos de la submission:")
			fmt.Println("  ID:", submissionID)
			fmt.Println("  Tipo:", submissionType)
			fmt.Println("  Datos:", string(submissionData))
			fmt.Println("  Estado:", submissionStatus)
			fmt.Println("  UserID:", submissionUserID)
			if submissionComment.Valid {
				fmt.Println("  Comentario:", submissionComment.String)
			} else {
				fmt.Println("  Comentario: <sin comentario>")
			}
		}
	}

	switch submissionType {
	case "band":
		var band Band
		if err := json.Unmarshal(dataRaw, &band); err != nil {
			http.Error(w, "Error al parsear datos", http.StatusBadRequest)
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

		// Crear vinculación automática entre el usuario que envió la banda y la banda creada
		if submissionUserID > 0 && newID > 0 {
			fmt.Println("Creando vinculación automática: UserID=", submissionUserID, "BandID=", newID)
			_, linkErr := h.DB.Insert(false, `INSERT INTO artist_links (user_id, artist_id, rol, status) VALUES (?, ?, ?, 'approved')`,
				submissionUserID, newID, "creador")
			if linkErr != nil {
				fmt.Println("Error al crear vinculación automática:", linkErr)
				// No interrumpimos el flujo principal, solo registramos el error
			} else {
				fmt.Println("Vinculación automática creada correctamente")
			}
		}

		// Devolver el ID de la banda creada para que DirectApprove pueda usarlo
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"band_id": newID,
		})

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

	case "eventvenue":
		fmt.Printf("[Info] Procesando submission tipo eventvenue (ID: %d)\n", id)

		var combined struct {
			Venue struct {
				Name        string `json:"name"`
				Address     string `json:"address"`
				Description string `json:"description"`
				Slug        string `json:"slug"`
				LatLng      string `json:"latlng"`
				City        string `json:"city"`
			} `json:"venue"`
			Event struct {
				Title     string `json:"title"`
				Slug      string `json:"slug"`
				Tags      string `json:"tags"`
				Content   string `json:"content"`
				DateStart string `json:"date_start"`
				DateEnd   string `json:"date_end"`
				BandIDs   []int  `json:"band_ids"`
			} `json:"event"`
		}

		if err := json.Unmarshal(dataRaw, &combined); err != nil {
			fmt.Printf("[Error] No se pudo decodificar el evento+venue: %v\n", err)
			fmt.Printf("[Error] Datos raw recibidos: %s\n", string(dataRaw))
			http.Error(w, "Error al parsear datos", http.StatusBadRequest)
			return
		}

		fmt.Printf("[Info] Datos decodificados correctamente: Venue=%s, Event=%s\n",
			combined.Venue.Name, combined.Event.Title)

		// Primero insertar el venue
		venueID, err := h.DB.Insert(false, `
			INSERT INTO venues (name, address, description, slug, latlng, city)
			VALUES (?, ?, ?, ?, ?, ?)`,
			combined.Venue.Name, combined.Venue.Address, combined.Venue.Description,
			combined.Venue.Slug, combined.Venue.LatLng, combined.Venue.City)
		if err != nil {
			fmt.Printf("[Error] No se pudo insertar el venue combinado: %v\n", err)
			http.Error(w, "Error al crear venue", http.StatusInternalServerError)
			return
		}

		fmt.Printf("[Info] Venue creado con ID: %d\n", venueID)

		// Vincular el venue con el usuario
		_, venLinkErr := h.DB.Insert(false, `INSERT INTO venue_links (user_id, venue_id, rol, status) VALUES (?, ?, ?, 'approved')`,
			submissionUserID, venueID, "creador")
		if venLinkErr != nil {
			fmt.Printf("[Error] Error al vincular venue %d con usuario %d: %v\n", venueID, submissionUserID, venLinkErr)
		}

		// Luego insertar el evento usando el ID del venue
		eventID, err := h.DB.Insert(false, `
			INSERT INTO events (id_venue, title, tags, content, slug, date_start, date_end)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			venueID, combined.Event.Title, combined.Event.Tags, combined.Event.Content,
			combined.Event.Slug, combined.Event.DateStart, combined.Event.DateEnd)
		if err != nil {
			fmt.Printf("[Error] No se pudo insertar el evento combinado: %v\n", err)
			http.Error(w, "Error al crear evento", http.StatusInternalServerError)
			return
		}

		fmt.Printf("[Info] Evento creado con ID: %d\n", eventID)

		// Insertar relaciones en events_bands
		for _, bandID := range combined.Event.BandIDs {
			_, err := h.DB.Insert(false, `INSERT INTO events_bands (id_band, id_event) VALUES (?, ?)`,
				bandID, eventID)
			if err != nil {
				fmt.Printf("[Error] Error insertando banda %d: %v\n", bandID, err)
			}
		}

		_ = moveImageInSpaces("pending/"+combined.Event.Slug+".jpg", "events/"+combined.Event.Slug+".jpg")
		_, _ = h.DB.Update(false, `UPDATE submissions SET status = 'approved', reviewed_by = ? WHERE id = ?`, payload.ReviewerID, id)

		// Crear vinculación automática entre el usuario que envió el evento y el evento creado
		if submissionUserID > 0 && eventID > 0 {
			fmt.Println("Creando vinculación automática para evento: UserID=", submissionUserID, "EventID=", eventID)
			_, linkErr := h.DB.Insert(false, `INSERT INTO event_links (user_id, event_id, rol, status) VALUES (?, ?, ?, 'approved')`,
				submissionUserID, eventID, "creador")
			if linkErr != nil {
				fmt.Println("Error al crear vinculación automática para evento:", linkErr)
				// No interrumpimos el flujo principal, solo registramos el error
			} else {
				fmt.Println("Vinculación automática para evento creada correctamente")
			}
		}

		// Publicar en redes sociales
		imagePath := "events/" + combined.Event.Slug + ".jpg"
		go func() {
			err := h.PublishToSocial("eventvenue", combined, imagePath)
			idInt, _ := strconv.Atoi(id)
			if err != nil {
				h.LogSocialActivity(idInt, "eventvenue", false, err.Error())
			} else {
				h.LogSocialActivity(idInt, "eventvenue", true, "")
			}
		}()

		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "event_id": eventID, "venue_id": venueID})

	case "event":
		var eventData map[string]interface{}
		fmt.Println("dataRaw:", string(dataRaw))

		if err := json.Unmarshal(dataRaw, &eventData); err != nil {
			http.Error(w, "Error al parsear datos: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Crear un evento con los datos disponibles
		event := Event{
			Title:     eventData["title"].(string),
			Slug:      eventData["slug"].(string),
			Tags:      eventData["tags"].(string),
			Content:   eventData["content"].(string),
			DateStart: eventData["date_start"].(string),
			DateEnd:   eventData["date_end"].(string),
		}

		// Manejar el id_venue que puede estar vacío
		var venueID int
		if idVenue, ok := eventData["id_venue"].(string); ok && idVenue != "" {
			venueID, _ = strconv.Atoi(idVenue)
		} else if idVenue, ok := eventData["id_venue"].(float64); ok {
			venueID = int(idVenue)
		} else {
			// Si no hay venue, usar un venue por defecto o mostrar error
			fmt.Println("Advertencia: id_venue está vacío o no es válido, usando venue por defecto")
			// Aquí podrías usar un venue por defecto o crear uno nuevo
			// Por ahora, usaremos el ID 1 como venue por defecto
			venueID = 1
		}

		fmt.Println("Usando venueID:", venueID)

		eventID, err := h.DB.Insert(false, `
		INSERT INTO events (id_venue, title, tags, content, slug, date_start, date_end)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
			venueID, event.Title, event.Tags, event.Content, event.Slug, event.DateStart, event.DateEnd)

		if err != nil {
			http.Error(w, "Error al crear evento: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Manejar band_ids que puede ser un array
		if bandIDs, ok := eventData["band_ids"].([]interface{}); ok {
			for _, bandID := range bandIDs {
				var bandIDInt int
				switch v := bandID.(type) {
				case float64:
					bandIDInt = int(v)
				case string:
					bandIDInt, _ = strconv.Atoi(v)
				case int:
					bandIDInt = v
				}

				if bandIDInt > 0 {
					_, err := h.DB.Insert(false, `INSERT INTO events_bands (id_event, id_band) VALUES (?, ?)`,
						eventID, bandIDInt)
					if err != nil {
						fmt.Println("Error al vincular banda", bandIDInt, "al evento", eventID, ":", err)
					}
				}
			}
		}

		_ = moveImageInSpaces("pending/"+event.Slug+".jpg", "events/"+event.Slug+".jpg")
		_, _ = h.DB.Update(false, `UPDATE submissions SET status = 'approved', reviewed_by = ? WHERE id = ?`, payload.ReviewerID, id)

		// print submissionUserID and eventID
		fmt.Println("submissionUserID:", submissionUserID)
		fmt.Println("eventID:", eventID)

		// Crear vinculación automática entre el usuario que envió el evento y el evento creado
		if submissionUserID > 0 && eventID > 0 {
			fmt.Println("Creando vinculación automática para evento: UserID=", submissionUserID, "EventID=", eventID)
			_, linkErr := h.DB.Insert(false, `INSERT INTO event_links (user_id, event_id, rol, status) VALUES (?, ?, ?, 'approved')`,
				submissionUserID, eventID, "creador")
			if linkErr != nil {
				fmt.Println("Error al crear vinculación automática para evento:", linkErr)
				// No interrumpimos el flujo principal, solo registramos el error
			} else {
				fmt.Println("Vinculación automática para evento creada correctamente")
			}
		}

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

	case "artist_link":
		// Imprimir los datos raw para depuración
		fmt.Println("Datos raw de artist_link:", string(dataRaw))

		// Definir la estructura para almacenar los datos de la vinculación
		var link struct {
			ArtistID int
			UserID   int
			Name     string
			Slug     string
			Rol      string
		}

		// Primero, intentar extraer directamente usando una estructura definida
		var submissionData struct {
			Data struct {
				ArtistID int    `json:"artist_id"`
				Name     string `json:"name"`
				Slug     string `json:"slug"`
				Rol      string `json:"rol"`
			} `json:"data"`
		}

		// También probar con una estructura alternativa donde los datos están en el nivel superior
		var directData struct {
			ArtistID int    `json:"artist_id"`
			Name     string `json:"name"`
			Slug     string `json:"slug"`
			Rol      string `json:"rol"`
		}

		// Intentar primero con la estructura anidada
		if err := json.Unmarshal(dataRaw, &submissionData); err == nil && submissionData.Data.ArtistID > 0 {
			link.ArtistID = submissionData.Data.ArtistID
			link.Name = submissionData.Data.Name
			link.Slug = submissionData.Data.Slug
			link.Rol = submissionData.Data.Rol
			fmt.Println("Datos extraídos de estructura anidada: ArtistID=", link.ArtistID)
		} else if err := json.Unmarshal(dataRaw, &directData); err == nil && directData.ArtistID > 0 {
			// Si falla, intentar con la estructura directa
			link.ArtistID = directData.ArtistID
			link.Name = directData.Name
			link.Slug = directData.Slug
			link.Rol = directData.Rol
			fmt.Println("Datos extraídos de estructura directa: ArtistID=", link.ArtistID)
		} else {
			fmt.Println("No se pudieron extraer datos de las estructuras JSON:", err)
		}

		// Si los IDs siguen siendo inválidos, intentar obtenerlos directamente de la base de datos
		if link.UserID <= 0 || link.ArtistID <= 0 {
			// Obtener los datos completos de la submission
			var submissionRaw []byte
			rawDataRow, _ := h.DB.SelectRow("SELECT data FROM submissions WHERE id = ?", id)
			if rawDataRow != nil && rawDataRow.Scan(&submissionRaw) == nil {
				// Intentar extraer directamente el artist_id
				var rawData map[string]interface{}
				if json.Unmarshal(submissionRaw, &rawData) == nil {
					if dataObj, ok := rawData["data"].(map[string]interface{}); ok {
						if artistID, ok := dataObj["artist_id"]; ok {
							switch v := artistID.(type) {
							case float64:
								link.ArtistID = int(v)
							case int:
								link.ArtistID = v
							case string:
								if id, err := strconv.Atoi(v); err == nil {
									link.ArtistID = id
								}
							}
						}
					}
				}

				// Obtener el user_id directamente
				var userID int
				userIDRow, _ := h.DB.SelectRow("SELECT user_id FROM submissions WHERE id = ?", id)
				if userIDRow != nil && userIDRow.Scan(&userID) == nil && userID > 0 {
					link.UserID = userID
				}
			}

			fmt.Println("Datos finales para la vinculación:", link)
		}

		// Verificar que los IDs sean válidos
		if link.UserID <= 0 || link.ArtistID <= 0 {
			errMsg := fmt.Sprintf("IDs inválidos: UserID=%d, ArtistID=%d", link.UserID, link.ArtistID)
			fmt.Println(errMsg)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

		// Verificar si ya existe una vinculación
		var exists bool
		existsRow, _ := h.DB.SelectRow("SELECT EXISTS(SELECT 1 FROM artist_links WHERE user_id = ? AND artist_id = ?)",
			link.UserID, link.ArtistID)
		if err := existsRow.Scan(&exists); err == nil && exists {
			// Actualizar en lugar de insertar
			_, err = h.DB.Update(false, `UPDATE artist_links SET rol = ?, status = 'approved' WHERE user_id = ? AND artist_id = ?`,
				link.Rol, link.UserID, link.ArtistID)
		} else {
			// Insertar nueva vinculación
			_, err = h.DB.Insert(false, `INSERT INTO artist_links (user_id, artist_id, rol, status) VALUES (?, ?, ?, 'approved')`,
				link.UserID, link.ArtistID, link.Rol)
			fmt.Printf("Creando nueva vinculación de artista: UserID=%d, ArtistID=%d, Rol=%s\n",
				link.UserID, link.ArtistID, link.Rol)
		}

		if err != nil {
			fmt.Println("Error al crear/actualizar vínculo de artista:", err)
			http.Error(w, "Error al crear vínculo de artista: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Actualizar estado de la submission
		updateResult, err := h.DB.Update(false, `UPDATE submissions SET status = 'approved', reviewed_by = ? WHERE id = ?`,
			payload.ReviewerID, id)
		if err != nil {
			fmt.Println("Error al actualizar estado de submission:", err)
		} else {
			fmt.Println("Submission actualizada correctamente, filas afectadas:", updateResult)
		}

		// Enviar notificación por WhatsApp si hay un número guardado
		comment, _ := h.DB.SelectRow("SELECT comment FROM submissions WHERE id = ?", id)
		var commentStr string
		if err := comment.Scan(&commentStr); err != nil {
			fmt.Println("Error al obtener comentario:", err)
			// No interrumpimos el flujo, solo registramos el error
		}

		if strings.Contains(commentStr, "WhatsApp:") {
			// Extraer número de WhatsApp
			whatsappParts := strings.Split(commentStr, "WhatsApp:")
			if len(whatsappParts) > 1 {
				userPhone := strings.TrimSpace(whatsappParts[1])
				if userPhone != "" {
					// Cargar configuración
					cfg, err := ini.Load("data.conf")
					if err == nil {
						// Preparar datos para el mensaje
						fmt.Println("Preparando mensaje de WhatsApp para:", userPhone)
						fmt.Println("Datos de la vinculación - Rol:", link.Rol, "Nombre:", link.Name)

						// Asegurar que los valores no estén vacíos
						rol := link.Rol
						if rol == "" {
							rol = "colaborador"
						}

						artistName := link.Name
						if artistName == "" {
							artistName = "el artista"
						}

						// Verificar formato del número de teléfono
						if !strings.HasPrefix(userPhone, "+") && !strings.HasPrefix(userPhone, "549") {
							// Agregar prefijo de Argentina si no lo tiene
							userPhone = "549" + userPhone
						}

						// Eliminar cualquier espacio o carácter no numérico (excepto el +)
						userPhone = strings.Map(func(r rune) rune {
							if r == '+' || (r >= '0' && r <= '9') {
								return r
							}
							return -1
						}, userPhone)

						fmt.Println("Número de teléfono formateado:", userPhone)

						// Construir el mensaje según la plantilla vinculacion_aprobada
						message := map[string]interface{}{
							"messaging_product": "whatsapp",
							"recipient_type":    "individual",
							"to":                userPhone,
							"type":              "template",
							"template": map[string]interface{}{
								"name": "vinculacion_aprobada",
								"language": map[string]interface{}{
									"code": "es",
								},
								"components": []map[string]interface{}{
									{
										"type": "body",
										"parameters": []map[string]interface{}{
											{"type": "text", "text": rol},
											{"type": "text", "text": artistName},
										},
									},
								},
							},
						}

						// Enviar mensaje
						whatsappToken := cfg.Section("keys").Key("whatsapp_token").String()
						whatsappPhoneID := cfg.Section("keys").Key("whatsapp_number").String()

						if whatsappToken != "" && whatsappPhoneID != "" {
							go func() {
								jsonData, _ := json.Marshal(message)
								fmt.Println("Enviando WhatsApp a:", userPhone)
								fmt.Println("Datos del mensaje:", string(jsonData))

								req, _ := http.NewRequest("POST", fmt.Sprintf("https://graph.facebook.com/v17.0/%s/messages", whatsappPhoneID), bytes.NewBuffer(jsonData))
								req.Header.Set("Content-Type", "application/json")
								req.Header.Set("Authorization", "Bearer "+whatsappToken)

								client := &http.Client{
									Timeout: 30 * time.Second, // Agregar timeout para evitar bloqueos indefinidos
								}
								resp, err := client.Do(req)
								if err != nil {
									fmt.Println("Error al enviar WhatsApp:", err)
									return
								}

								// Leer y mostrar la respuesta
								defer resp.Body.Close()
								respBody, _ := io.ReadAll(resp.Body)
								fmt.Println("Respuesta de WhatsApp API:", resp.Status)
								fmt.Println(string(respBody))

								// Si hay un error, intentar interpretar la respuesta
								if resp.StatusCode >= 400 {
									var errorResp map[string]interface{}
									if err := json.Unmarshal(respBody, &errorResp); err == nil {
										if errObj, ok := errorResp["error"].(map[string]interface{}); ok {
											fmt.Println("Código de error:", errObj["code"])
											fmt.Println("Mensaje de error:", errObj["message"])
											if errData, ok := errObj["error_data"].(map[string]interface{}); ok {
												fmt.Println("Detalles del error:", errData["details"])
											}
										}
									}
								}
							}()
						} else {
							fmt.Println("No se pudo enviar WhatsApp: falta token o phone_id")
							fmt.Println("Phone ID:", whatsappPhoneID)
							fmt.Println("Token disponible:", whatsappToken != "")
						}
					} else {
						fmt.Println("Error al cargar configuración para WhatsApp:", err)
					}
				} else {
					fmt.Println("Número de WhatsApp vacío")
				}
			} else {
				fmt.Println("Formato incorrecto para WhatsApp en el comentario")
			}
		} else {
			fmt.Println("No se encontró número de WhatsApp en el comentario")
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
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

	// Determinar la carpeta de destino (pending por defecto, o la especificada)
	destination := r.FormValue("destination")
	var key string

	if destination != "" {
		// Si se especifica un destino, usar ese (events, bands, venues, etc.)
		key = fmt.Sprintf("%s/%s.jpg", destination, slug)
		fmt.Printf("Guardando imagen en carpeta específica: %s\n", key)
	} else {
		// Por defecto, guardar en pending
		key = fmt.Sprintf("pending/%s.jpg", slug)
		fmt.Printf("Guardando imagen en carpeta pending: %s\n", key)
	}

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

	// Verificar si el usuario es administrador
	var isAdmin bool
	userRow, err := h.DB.SelectRow("SELECT role FROM users WHERE id = ?", s.UserID)
	if err == nil {
		var role string
		if err := userRow.Scan(&role); err == nil {
			isAdmin = role == "admin"
		}
	}

	// Determinar el estado inicial de la submission
	initialStatus := "pending"
	if isAdmin {
		// Si es admin, marcar como aprobada directamente
		initialStatus = "approved"
	}

	// Insertar la submission (sin reviewer_id que no existe en la tabla)
	id, err := h.DB.Insert(false, `
		INSERT INTO submissions (user_id, type, data, status, updated_at)
		VALUES (?, ?, ?, ?, NOW())`,
		s.UserID, s.Type, string(s.Data), initialStatus)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.ID = int(id)

	// Si es admin, actualizar el campo reviewed_by (que es el nombre correcto de la columna)
	if isAdmin {
		_, err = h.DB.Update(false, `
			UPDATE submissions 
			SET reviewed_by = ? 
			WHERE id = ?`,
			s.UserID, s.ID)
		if err != nil {
			fmt.Printf("[Warning] No se pudo actualizar el campo reviewed_by: %v\n", err)
			// No interrumpimos el flujo por este error
		}
	}

	// asignar el RawMessage para poder usar extractFields
	s.Data = json.RawMessage(s.Data)

	// cargar configuración desde data.conf
	cfg, err := ini.Load("data.conf")
	if err == nil {
		// extraer campos y mandar WhatsApp
		name, description, slug, err := extractFieldsFromSubmission(s)

		// print name, description, slug
		fmt.Printf("[Debug] CreateSubmission - name=%s, description=%s, slug=%s\n", name, description, slug)

		if err == nil {
			// número del admin al que querés mandar el mensaje
			adminPhone := cfg.Section("keys").Key("admin_phone").String()
			if adminPhone != "" && !isAdmin { // No enviar WhatsApp si es un admin
				sendSubmissionWhatsApp(adminPhone, s, name, description, slug, cfg)
			}

			// Si es una solicitud de vinculación y se proporcionó un número de WhatsApp, guardarlo
			if s.Type == "artist_link" {
				var linkData struct {
					WhatsApp string `json:"whatsapp"`
				}
				if err := json.Unmarshal(s.Data, &linkData); err == nil && linkData.WhatsApp != "" {
					// Guardar el número de WhatsApp en la base de datos para notificaciones futuras
					_, _ = h.DB.Update(false, `
						UPDATE submissions 
						SET comment = ? 
						WHERE id = ?`,
						fmt.Sprintf("WhatsApp: %s", linkData.WhatsApp), s.ID)
				}
			}
		}
	}

	// Si es administrador, procesar la submission automáticamente
	if isAdmin {
		go func(submissionID int, submissionType string, submissionData json.RawMessage) {
			// Esperar un momento para asegurar que la imagen se haya procesado
			time.Sleep(2 * time.Second)

			fmt.Printf("[Info] Iniciando procesamiento automático para submission %d (admin) - tipo: %s\n",
				submissionID, submissionType)

			// Procesar la submission aprobada directamente
			success := h.processApprovedSubmission(submissionID, s.UserID)

			if success {
				// Eliminar la submission después de procesarla
				_, err = h.DB.Delete(false, "DELETE FROM submissions WHERE id = ?", submissionID)
				if err != nil {
					fmt.Printf("[Error] No se pudo eliminar la submission %d después de procesarla: %v\n", submissionID, err)
				} else {
					fmt.Printf("[Info] Submission %d procesada y eliminada correctamente\n", submissionID)
				}
			} else {
				fmt.Printf("[Warning] No se eliminó la submission %d porque hubo errores en el procesamiento\n", submissionID)
			}
		}(s.ID, s.Type, s.Data)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"id":     s.ID,
	})
}

func (h *AuthHandler) UpdateSubmissionStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		Status     string          `json:"status"`
		Comment    sql.NullString  `json:"comment"` // Cambiado a sql.NullString para manejar NULL
		ReviewerID int             `json:"reviewer_id"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Error al decodificar", http.StatusBadRequest)
		return
	}

	// Verificar que el estado sea válido
	validStatus := map[string]bool{
		"pending":  true,
		"approved": true,
		"rejected": true,
	}
	if !validStatus[payload.Status] {
		http.Error(w, "Estado no válido", http.StatusBadRequest)
		return
	}

	// Procesar el comentario para asegurarnos que sea un NullString válido
	if payload.Comment.String == "" {
		payload.Comment.Valid = false
	} else {
		payload.Comment.Valid = true
	}

	// Si el estado es "approved", crear un vínculo de artista
	if payload.Status == "approved" {
		// Obtener datos de la submission
		row, err := h.DB.SelectRow("SELECT type, data, user_id FROM submissions WHERE id = ?", id)
		if err != nil {
			http.Error(w, "Error al obtener submission", http.StatusInternalServerError)
			return
		}

		var submissionType string
		var dataRaw json.RawMessage
		var userID int
		if err := row.Scan(&submissionType, &dataRaw, &userID); err != nil {
			http.Error(w, "Error al leer datos de submission", http.StatusInternalServerError)
			return
		}

		fmt.Println("Procesando aprobación de submission tipo:", submissionType, "UserID:", userID)
		fmt.Println("Datos raw:", string(dataRaw))

		// Verificar si es una solicitud de vinculación
		if submissionType == "artist_link" {
			// Extraer datos de la solicitud de vinculación
			var link struct {
				ArtistID int
				UserID   int
				Name     string
				Slug     string
				Rol      string
			}

			// Asignar el user_id que ya obtuvimos de la base de datos
			link.UserID = userID

			// Intentar extraer datos de la estructura JSON
			var submissionData struct {
				Data struct {
					ArtistID int    `json:"artist_id"`
					Name     string `json:"name"`
					Slug     string `json:"slug"`
					Rol      string `json:"rol"`
				} `json:"data"`
			}

			if err := json.Unmarshal(dataRaw, &submissionData); err == nil {
				link.ArtistID = submissionData.Data.ArtistID
				link.Name = submissionData.Data.Name
				link.Slug = submissionData.Data.Slug
				link.Rol = submissionData.Data.Rol
			}

			// Verificar que los IDs sean válidos
			if link.UserID <= 0 || link.ArtistID <= 0 {
				http.Error(w, "IDs inválidos para vinculación de artista", http.StatusBadRequest)
				return
			}

			// Verificar si ya existe una vinculación
			var exists bool
			existsRow, _ := h.DB.SelectRow("SELECT EXISTS(SELECT 1 FROM artist_links WHERE user_id = ? AND artist_id = ?)",
				link.UserID, link.ArtistID)
			if err := existsRow.Scan(&exists); err == nil && exists {
				// Actualizar en lugar de insertar
				_, err = h.DB.Update(false, `UPDATE artist_links SET rol = ?, status = 'approved' WHERE user_id = ? AND artist_id = ?`,
					link.Rol, link.UserID, link.ArtistID)
			} else {
				// Insertar nueva vinculación
				_, err = h.DB.Insert(false, `INSERT INTO artist_links (user_id, artist_id, rol, status) VALUES (?, ?, ?, 'approved')`,
					link.UserID, link.ArtistID, link.Rol)
			}

			if err != nil {
				http.Error(w, "Error al crear vínculo de artista", http.StatusInternalServerError)
				return
			}
		}
	}

	// Actualizar la submission en la base de datos
	_, err := h.DB.Update(false, `
		UPDATE submissions SET status=?, comment=?, reviewed_by=?, data=? WHERE id=?`,
		payload.Status, payload.Comment, payload.ReviewerID, payload.Data, id)
	if err != nil {
		fmt.Println("Error al actualizar submission:", err)
		http.Error(w, "Error al actualizar submission: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Enviar notificación de WhatsApp si es necesario
	if payload.Status == "approved" || payload.Status == "rejected" {
		// Obtener datos de la submission
		var (
			userID       int
			submissionID int
			whatsapp     string
			comment      sql.NullString // Cambiado a sql.NullString para manejar NULL
		)

		// Obtener el número de WhatsApp del usuario
		row, err := h.DB.SelectRow(`
			SELECT s.id, s.user_id, u.whatsapp, s.comment
			FROM submissions s
			JOIN users u ON s.user_id = u.id
			WHERE s.id = ?
		`, id)

		if err == nil && row != nil {
			if err := row.Scan(&submissionID, &userID, &whatsapp, &comment); err == nil && whatsapp != "" {
				// Preparar mensaje según el estado
				var message string
				if payload.Status == "approved" {
					message = fmt.Sprintf("¡Buenas noticias! Tu solicitud en Brote Colectivo ha sido aprobada.")
				} else {
					message = fmt.Sprintf("Tu solicitud en Brote Colectivo ha sido rechazada.")
				}

				// Agregar comentario si existe
				if comment.Valid && comment.String != "" {
					message += fmt.Sprintf("\n\nComentario: %s", comment.String)
				}

				// Enviar mensaje de WhatsApp
				go h.sendWhatsAppMessage(whatsapp, message)
			} else if err != nil {
				fmt.Println("Error al escanear datos para WhatsApp:", err)
			}
		} else if err != nil {
			fmt.Println("Error al obtener datos para WhatsApp:", err)
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
}

// sendWhatsAppMessage envía un mensaje simple de WhatsApp al número especificado
func (h *AuthHandler) sendWhatsAppMessage(phone, message string) error {
	// Cargar configuración
	cfg, err := ini.Load("data.conf")
	if err != nil {
		fmt.Println("Error al cargar configuración para WhatsApp:", err)
		return err
	}

	// Obtener credenciales de WhatsApp
	whatsappNumber := cfg.Section("keys").Key("whatsapp_number").String()
	whatsappToken := cfg.Section("keys").Key("whatsapp_token").String()

	// Verificar que tengamos las credenciales necesarias
	if whatsappNumber == "" || whatsappToken == "" {
		errMsg := "Faltan credenciales de WhatsApp en la configuración"
		fmt.Println(errMsg)
		return fmt.Errorf(errMsg)
	}

	// Preparar el payload para el mensaje de texto simple
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                phone,
		"type":              "text",
		"text": map[string]string{
			"body": message,
		},
	}

	// Convertir a JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		errMsg := fmt.Sprintf("Error al convertir mensaje a JSON: %v", err)
		fmt.Println(errMsg)
		return fmt.Errorf(errMsg)
	}

	// Preparar la solicitud HTTP
	apiURL := fmt.Sprintf("https://graph.facebook.com/v17.0/%s/messages", whatsappNumber)
	fmt.Printf("[WhatsApp Debug] URL de API: %s\n", apiURL)

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		errMsg := fmt.Sprintf("Error al crear request HTTP: %v", err)
		fmt.Println(errMsg)
		return fmt.Errorf(errMsg)
	}

	// Configurar headers
	req.Header.Set("Authorization", "Bearer "+whatsappToken)
	req.Header.Set("Content-Type", "application/json")

	// Enviar la solicitud
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	fmt.Println("[WhatsApp Debug] Enviando request a la API de WhatsApp...")
	resp, err := client.Do(req)
	if err != nil {
		errMsg := fmt.Sprintf("Error al enviar request: %v", err)
		fmt.Println(errMsg)
		return fmt.Errorf(errMsg)
	}
	defer resp.Body.Close()

	// Leer y mostrar la respuesta
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[WhatsApp Debug] Respuesta de API WhatsApp (status=%d): %s\n", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errMsg := fmt.Sprintf("WhatsApp API error: %s", string(body))
		fmt.Println("[WhatsApp Debug] " + errMsg)
		return fmt.Errorf(errMsg)
	}

	fmt.Println("[WhatsApp Debug] Mensaje enviado exitosamente")
	return nil
}

// processApprovedSubmission procesa una submission aprobada y crea el contenido correspondiente
// Devuelve true si el procesamiento fue exitoso, false en caso contrario
func (h *AuthHandler) processApprovedSubmission(submissionID, reviewerID int) bool {
	fmt.Printf("[Info] Procesando automáticamente submission %d\n", submissionID)

	// Obtener datos de la submission
	row, err := h.DB.SelectRow("SELECT type, data, user_id FROM submissions WHERE id = ?", submissionID)
	if err != nil {
		fmt.Printf("[Error] No se pudo obtener la submission %d: %v\n", submissionID, err)
		return false
	}

	var submissionType string
	var dataRaw []byte
	var userID int
	if err := row.Scan(&submissionType, &dataRaw, &userID); err != nil {
		fmt.Printf("[Error] No se pudo leer los datos de la submission %d: %v\n", submissionID, err)
		return false
	}

	fmt.Printf("[Info] Procesando submission tipo: %s (datos: %s)\n", submissionType, string(dataRaw))

	success := false

	switch submissionType {
	case "event":
		success = h.processEventSubmission(dataRaw, userID)
	case "venue":
		success = h.processVenueSubmission(dataRaw, userID)
	case "eventvenue":
		success = h.processEventVenueSubmission(dataRaw, userID)
	default:
		fmt.Printf("[Warning] Tipo de submission no soportado para procesamiento automático: %s\n", submissionType)
		return false
	}

	return success
}

// processEventSubmission procesa una submission de tipo event
func (h *AuthHandler) processEventSubmission(dataRaw []byte, userID int) bool {
	// Procesar evento
	var event struct {
		Title     string `json:"title"`
		Tags      string `json:"tags"`
		Content   string `json:"content"`
		Slug      string `json:"slug"`
		DateStart string `json:"date_start"`
		DateEnd   string `json:"date_end"`
		IDVenue   int    `json:"id_venue"`
		BandIDs   []int  `json:"band_ids"`
	}

	if err := json.Unmarshal(dataRaw, &event); err != nil {
		fmt.Printf("[Error] No se pudo decodificar el evento: %v\n", err)
		return false
	}

	fmt.Printf("[Info] Datos de evento decodificados: %s (slug: %s)\n", event.Title, event.Slug)

	// Insertar el evento
	eventID, err := h.DB.Insert(false, `
		INSERT INTO events (id_venue, title, tags, content, slug, date_start, date_end)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.IDVenue, event.Title, event.Tags, event.Content, event.Slug, event.DateStart, event.DateEnd)
	if err != nil {
		fmt.Printf("[Error] No se pudo insertar el evento: %v\n", err)
		return false
	}

	// Insertar relaciones en events_bands
	for _, bandID := range event.BandIDs {
		_, err := h.DB.Insert(false, `
			INSERT INTO events_bands (id_band, id_event) VALUES (?, ?)`,
			bandID, eventID)
		if err != nil {
			fmt.Printf("[Error] Error insertando banda %d: %v\n", bandID, err)
		}
	}

	// Vincular el evento con el usuario que lo creó
	_, linkErr := h.DB.Insert(false, `
		INSERT INTO event_links (user_id, event_id, rol, status) 
		VALUES (?, ?, 'creador', 'approved')`,
		userID, eventID)
	if linkErr != nil {
		fmt.Printf("[Error] Error al vincular evento %d con usuario %d: %v\n", eventID, userID, linkErr)
	}

	fmt.Printf("[Success] Evento creado automáticamente con ID: %d\n", eventID)
	return true
}

// processVenueSubmission procesa una submission de tipo venue
func (h *AuthHandler) processVenueSubmission(dataRaw []byte, userID int) bool {
	// Procesar venue
	var venue struct {
		Name        string `json:"name"`
		Address     string `json:"address"`
		Description string `json:"description"`
		Slug        string `json:"slug"`
		LatLng      string `json:"latlng"`
		City        string `json:"city"`
	}

	if err := json.Unmarshal(dataRaw, &venue); err != nil {
		fmt.Printf("[Error] No se pudo decodificar el venue: %v\n", err)
		return false
	}

	fmt.Printf("[Info] Datos de venue decodificados: %s (slug: %s)\n", venue.Name, venue.Slug)

	// Insertar el venue
	venueID, err := h.DB.Insert(false, `
		INSERT INTO venues (name, address, description, slug, latlng, city)
		VALUES (?, ?, ?, ?, ?, ?)`,
		venue.Name, venue.Address, venue.Description, venue.Slug, venue.LatLng, venue.City)
	if err != nil {
		fmt.Printf("[Error] No se pudo insertar el venue: %v\n", err)
		return false
	}

	// Vincular el venue con el usuario que lo creó
	_, linkErr := h.DB.Insert(false, `
		INSERT INTO venue_links (user_id, venue_id, rol, status) 
		VALUES (?, ?, 'creador', 'approved')`,
		userID, venueID)
	if linkErr != nil {
		fmt.Printf("[Error] Error al vincular venue %d con usuario %d: %v\n", venueID, userID, linkErr)
	}

	fmt.Printf("[Success] Venue creado automáticamente con ID: %d\n", venueID)
	return true
}

// processEventVenueSubmission procesa una submission de tipo eventvenue
func (h *AuthHandler) processEventVenueSubmission(dataRaw []byte, userID int) bool {
	fmt.Printf("[Info] Procesando submission tipo eventvenue\n")

	// Procesar evento y venue combinados
	var combined struct {
		Venue struct {
			Name        string `json:"name"`
			Address     string `json:"address"`
			Description string `json:"description"`
			Slug        string `json:"slug"`
			LatLng      string `json:"latlng"`
			City        string `json:"city"`
		} `json:"venue"`
		Event struct {
			Title     string `json:"title"`
			Slug      string `json:"slug"`
			Tags      string `json:"tags"`
			Content   string `json:"content"`
			DateStart string `json:"date_start"`
			DateEnd   string `json:"date_end"`
			BandIDs   []int  `json:"band_ids"`
		} `json:"event"`
	}

	if err := json.Unmarshal(dataRaw, &combined); err != nil {
		fmt.Printf("[Error] No se pudo decodificar el evento+venue: %v\n", err)
		fmt.Printf("[Error] Datos raw recibidos: %s\n", string(dataRaw))
		return false
	}

	fmt.Printf("[Info] Datos decodificados correctamente: Venue=%s, Event=%s\n",
		combined.Venue.Name, combined.Event.Title)

	// Primero insertar el venue
	venueID, err := h.DB.Insert(false, `
		INSERT INTO venues (name, address, description, slug, latlng, city)
		VALUES (?, ?, ?, ?, ?, ?)`,
		combined.Venue.Name, combined.Venue.Address, combined.Venue.Description,
		combined.Venue.Slug, combined.Venue.LatLng, combined.Venue.City)
	if err != nil {
		fmt.Printf("[Error] No se pudo insertar el venue combinado: %v\n", err)
		return false
	}

	fmt.Printf("[Info] Venue creado con ID: %d\n", venueID)

	// Vincular el venue con el usuario
	_, venLinkErr := h.DB.Insert(false, `
		INSERT INTO venue_links (user_id, venue_id, rol, status) 
		VALUES (?, ?, 'creador', 'approved')`,
		userID, venueID)
	if venLinkErr != nil {
		fmt.Printf("[Error] Error al vincular venue %d con usuario %d: %v\n", venueID, userID, venLinkErr)
	}

	// Luego insertar el evento usando el ID del venue
	eventID, err := h.DB.Insert(false, `
		INSERT INTO events (id_venue, title, tags, content, slug, date_start, date_end)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		venueID, combined.Event.Title, combined.Event.Tags, combined.Event.Content,
		combined.Event.Slug, combined.Event.DateStart, combined.Event.DateEnd)
	if err != nil {
		fmt.Printf("[Error] No se pudo insertar el evento combinado: %v\n", err)
		return false
	}

	fmt.Printf("[Info] Evento creado con ID: %d\n", eventID)

	// Insertar relaciones en events_bands
	for _, bandID := range combined.Event.BandIDs {
		_, err := h.DB.Insert(false, `
			INSERT INTO events_bands (id_band, id_event) VALUES (?, ?)`,
			bandID, eventID)
		if err != nil {
			fmt.Printf("[Error] Error insertando banda %d: %v\n", bandID, err)
		}
	}

	// Vincular el evento con el usuario
	_, evLinkErr := h.DB.Insert(false, `
		INSERT INTO event_links (user_id, event_id, rol, status) 
		VALUES (?, ?, 'creador', 'approved')`,
		userID, eventID)
	if evLinkErr != nil {
		fmt.Printf("[Error] Error al vincular evento %d con usuario %d: %v\n", eventID, userID, evLinkErr)
	}

	// Mover la imagen si existe
	err = moveImageInSpaces("pending/"+combined.Event.Slug+".jpg", "events/"+combined.Event.Slug+".jpg")
	if err != nil {
		fmt.Printf("[Warning] No se pudo mover la imagen para el evento %s: %v\n", combined.Event.Slug, err)
	} else {
		fmt.Printf("[Info] Imagen movida correctamente para el evento %s\n", combined.Event.Slug)
	}

	fmt.Printf("[Success] Evento+Venue creados automáticamente con IDs: %d, %d\n", eventID, venueID)
	return true
}
