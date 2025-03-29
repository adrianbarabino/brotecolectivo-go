package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

// SocialConfig contiene la configuración para las APIs de redes sociales
type SocialConfig struct {
	InstagramAccessToken string
	InstagramPageID      string
	FacebookAccessToken  string
	FacebookPageID       string
}

// LoadSocialConfig carga la configuración de redes sociales desde el archivo de configuración
func LoadSocialConfig() (*SocialConfig, error) {
	cfg, err := ini.Load("social_config.ini")
	if err != nil {
		return nil, fmt.Errorf("error al cargar configuración social: %v", err)
	}

	social := &SocialConfig{
		InstagramAccessToken: cfg.Section("instagram").Key("access_token").String(),
		InstagramPageID:      cfg.Section("instagram").Key("page_id").String(),
		FacebookAccessToken:  cfg.Section("facebook").Key("access_token").String(),
		FacebookPageID:       cfg.Section("facebook").Key("page_id").String(),
	}

	return social, nil
}

// PublishToSocial publica un submission aprobado en redes sociales
func (h *AuthHandler) PublishToSocial(submissionType string, data interface{}, imagePath string) error {
	config, err := LoadSocialConfig()
	if err != nil {
		return fmt.Errorf("error al cargar configuración social: %v", err)
	}

	// Extraer información según el tipo de submission
	var title, description string
	switch submissionType {
	case "event":
		if event, ok := data.(Event); ok {
			title = event.Title
			description = event.Content
		}
	case "eventvenue":
		if raw, ok := data.(struct {
			Venue map[string]interface{} `json:"venue"`
			Event map[string]interface{} `json:"event"`
		}); ok {
			title = raw.Event["title"].(string)
			description = raw.Event["content"].(string)
		}
	case "band":
		if band, ok := data.(Band); ok {
			title = band.Name
			description = band.Bio
		}
	case "news":
		if news, ok := data.(News); ok {
			title = news.Title
			description = news.Content
		}
	default:
		return fmt.Errorf("tipo de submission no soportado para publicación social: %s", submissionType)
	}

	// Publicar en Facebook Feed
	if err := publishToFacebookFeed(config, title, description, imagePath); err != nil {
		return fmt.Errorf("error al publicar en Facebook Feed: %v", err)
	}

	// Publicar en Facebook Story
	if err := publishToFacebookStory(config, title, imagePath); err != nil {
		return fmt.Errorf("error al publicar en Facebook Story: %v", err)
	}

	// Verificar si Instagram está configurado (token y page_id válidos)
	if isInstagramConfigured(config) {
		// Publicar en Instagram Feed
		if err := publishToInstagramFeed(config, title, description, imagePath); err != nil {
			// Solo log, no detener el proceso si Instagram falla
			fmt.Printf("Error al publicar en Instagram Feed: %v\n", err)
		}

		// Publicar en Instagram Story
		if err := publishToInstagramStory(config, title, imagePath); err != nil {
			// Solo log, no detener el proceso si Instagram falla
			fmt.Printf("Error al publicar en Instagram Story: %v\n", err)
		}
	} else {
		fmt.Println("Instagram no configurado o sin permisos. Omitiendo publicación en Instagram.")
	}

	return nil
}

// isInstagramConfigured verifica si la configuración de Instagram es válida
func isInstagramConfigured(config *SocialConfig) bool {
	// Verificar que existan token y page_id para Instagram
	if config.InstagramAccessToken == "" || config.InstagramPageID == "" {
		return false
	}

	// Verificar que no sean los valores por defecto
	if config.InstagramAccessToken == "TU_ACCESS_TOKEN_DE_INSTAGRAM" ||
		config.InstagramPageID == "TU_PAGE_ID_DE_INSTAGRAM" {
		return false
	}

	// Opcionalmente, hacer una llamada de prueba a la API para verificar permisos
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s?fields=username&access_token=%s",
		config.InstagramPageID, config.InstagramAccessToken)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error al verificar conexión con Instagram: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	// Verificar código de respuesta
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error de API de Instagram: Código %d\n", resp.StatusCode)
		return false
	}

	return true
}

// publishToInstagramFeed publica una imagen con texto en el feed de Instagram
func publishToInstagramFeed(config *SocialConfig, title, description, imagePath string) error {
	// 1. Primero subimos la imagen a Instagram
	imageID, err := uploadImageToInstagram(config, imagePath)
	if err != nil {
		return err
	}

	// 2. Creamos la publicación con la imagen
	caption := fmt.Sprintf("%s\n\n%s\n\n#brotecolectivo #música #eventos", title, description)

	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/media_publish", config.InstagramPageID)
	payload := map[string]string{
		"creation_id":  imageID,
		"caption":      caption,
		"access_token": config.InstagramAccessToken,
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error al publicar en Instagram: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error de API de Instagram: %s", string(body))
	}

	return nil
}

// publishToInstagramStory publica una historia en Instagram
func publishToInstagramStory(config *SocialConfig, title, imagePath string) error {
	// 1. Primero subimos la imagen a Instagram
	imageID, err := uploadImageToInstagram(config, imagePath)
	if err != nil {
		return err
	}

	// 2. Creamos la historia con la imagen
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/media_publish", config.InstagramPageID)
	payload := map[string]interface{}{
		"media_type":   "STORIES",
		"creation_id":  imageID,
		"access_token": config.InstagramAccessToken,
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error al publicar historia en Instagram: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error de API de Instagram para historia: %s", string(body))
	}

	return nil
}

// uploadImageToInstagram sube una imagen a Instagram y devuelve el ID de contenido
func uploadImageToInstagram(config *SocialConfig, imagePath string) (string, error) {
	// Verificar que la imagen existe
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return "", fmt.Errorf("la imagen no existe: %s", imagePath)
	}

	// Preparar la URL para subir la imagen
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/media", config.InstagramPageID)

	// Crear un buffer para el cuerpo multipart
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Agregar el archivo
	file, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("error al abrir imagen: %v", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("source", filepath.Base(imagePath))
	if err != nil {
		return "", fmt.Errorf("error al crear form file: %v", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return "", fmt.Errorf("error al copiar imagen: %v", err)
	}

	// Agregar otros campos
	_ = writer.WriteField("access_token", config.InstagramAccessToken)
	_ = writer.WriteField("media_type", "IMAGE")

	err = writer.Close()
	if err != nil {
		return "", fmt.Errorf("error al cerrar writer: %v", err)
	}

	// Hacer la solicitud HTTP
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", fmt.Errorf("error al crear request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error al enviar request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error de API de Instagram: %s", string(respBody))
	}

	// Leer la respuesta
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error al decodificar respuesta: %v", err)
	}

	return result.ID, nil
}

// uploadImageToFacebook sube una imagen a Facebook y devuelve el ID de contenido
func uploadImageToFacebook(config *SocialConfig, imagePath string) (string, error) {
	// Verificar que la imagen existe
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return "", fmt.Errorf("la imagen no existe: %s", imagePath)
	}

	// Preparar la URL para subir la imagen
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/photos", config.FacebookPageID)

	// Crear un buffer para el cuerpo multipart
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Agregar el archivo
	file, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("error al abrir imagen: %v", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("source", filepath.Base(imagePath))
	if err != nil {
		return "", fmt.Errorf("error al crear form file: %v", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return "", fmt.Errorf("error al copiar imagen: %v", err)
	}

	// Agregar otros campos
	_ = writer.WriteField("access_token", config.FacebookAccessToken)
	_ = writer.WriteField("published", "false") // Subimos pero no publicamos aún

	err = writer.Close()
	if err != nil {
		return "", fmt.Errorf("error al cerrar writer: %v", err)
	}

	// Hacer la solicitud HTTP
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", fmt.Errorf("error al crear request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error al enviar request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error de API de Facebook: %s", string(respBody))
	}

	// Leer la respuesta
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error al decodificar respuesta: %v", err)
	}

	return result.ID, nil
}

// publishToFacebookFeed publica una imagen con texto en el feed de Facebook
func publishToFacebookFeed(config *SocialConfig, title, description, imagePath string) error {
	// 1. Primero subimos la imagen a Facebook
	imageID, err := uploadImageToFacebook(config, imagePath)
	if err != nil {
		return err
	}

	// 2. Creamos la publicación con la imagen
	message := fmt.Sprintf("%s\n\n%s\n\n#brotecolectivo #música #eventos", title, description)

	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/feed", config.FacebookPageID)
	payload := map[string]string{
		"object_attachment": imageID,
		"message":           message,
		"access_token":      config.FacebookAccessToken,
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error al publicar en Facebook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error de API de Facebook: %s", string(body))
	}

	return nil
}

// publishToFacebookStory publica una historia en Facebook
func publishToFacebookStory(config *SocialConfig, title, imagePath string) error {
	// Verificar que la imagen existe
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return fmt.Errorf("la imagen no existe: %s", imagePath)
	}

	// URL para publicar una historia en Facebook
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/stories", config.FacebookPageID)

	// Crear un buffer para el cuerpo multipart
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Agregar el archivo
	file, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("error al abrir imagen: %v", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("source", filepath.Base(imagePath))
	if err != nil {
		return fmt.Errorf("error al crear form file: %v", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return fmt.Errorf("error al copiar imagen: %v", err)
	}

	// Agregar otros campos
	_ = writer.WriteField("access_token", config.FacebookAccessToken)
	_ = writer.WriteField("message", title) // Añadimos el título como mensaje

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("error al cerrar writer: %v", err)
	}

	// Hacer la solicitud HTTP
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("error al crear request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error al enviar request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error de API de Facebook para historia: %s", string(respBody))
	}

	return nil
}

// LogSocialActivity registra la actividad de publicación en redes sociales
func (h *AuthHandler) LogSocialActivity(submissionID int, submissionType string, success bool, errorMsg string) error {
	_, err := h.DB.Insert(false, `
		INSERT INTO social_activity_logs (submission_id, submission_type, success, error_message, created_at)
		VALUES (?, ?, ?, ?, NOW())
	`, submissionID, submissionType, success, errorMsg)

	return err
}
