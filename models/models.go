package models

import (
	"database/sql"
	"encoding/json"

	"github.com/golang-jwt/jwt"
)

type Item struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Category  string `json:"category"` // Asumiendo que este campo almacena el ID de la categoría como una cadena
	Price     int    `json:"price"`
	Image     string `json:"image"`
	Warehouse string `json:"warehouse"` // Asumiendo que este campo almacena el ID del almacén como una cadena
	Features  string `json:"features"`  // Este campo podría requerir un manejo especial para JSON
	Quantity  string `json:"quantity"`  // Asumiendo que este campo almacena información compleja como una cadena
	UpdatedAt string `json:"updated_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	Code      string `json:"code"`
	Active    int    `json:"active"`
}

type Client struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Code       string `json:"code,omitempty"` // El campo omitempty indica que el campo puede ser omitido si está vacío
	Address    string `json:"address"`
	Phone      string `json:"phone,omitempty"` // El campo omitempty indica que el campo puede ser omitido si está vacío
	Email      string `json:"email"`
	Web        string `json:"web,omitempty"`
	City       string `json:"city"`
	CategoryID int    `json:"category_id,omitempty"`
	Company    string `json:"company,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"` // Asume que este campo es manejado automáticamente por la base de datos
	UpdatedAt  string `json:"updated_at,omitempty"` // Asume que este campo es manejado automáticamente por la base de datos
}

type Contact struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Position    string `json:"position,omitempty"` // Omite si está vacío
	Phone       string `json:"phone,omitempty"`    // Omite si está vacío
	Email       string `json:"email"`
	ClientIDs   []int  `json:"client_ids,omitempty"`    // Omite si está vacío
	ProviderIDs []int  `json:"providers_ids,omitempty"` // Omite si está vacío

	CreatedAt string `json:"created_at,omitempty"` // Omite si está vacío, manejado por la DB
	UpdatedAt string `json:"updated_at,omitempty"` // Omite si está vacío, manejado por la DB
}

// ContactWithClients es una estructura extendida de Contact para incluir los ClientIDs asociados
type ContactWithClients struct {
	Contact         // Incorporación anónima de la estructura Contact
	ClientIDs []int `json:"client_ids"` // Slice de IDs de clientes
}

// ContactWithClients es una estructura extendida de Contact para incluir los ClientIDs asociados
type ContactWithClientsAndProviders struct {
	Contact           // Incorporación anónima de la estructura Contact
	ClientIDs   []int `json:"client_ids"`   // Slice de IDs de clientes
	ProviderIDs []int `json:"provider_ids"` // Slice de IDs de clientes
}

// ClientContact representa la relación muchos a muchos entre Contactos y Clientes
type ClientContact struct {
	ClientID  int `json:"client_id"`
	ContactID int `json:"contact_id"`
}

type Provider struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Code       string `json:"code,omitempty"` // El campo omitempty indica que el campo puede ser omitido si está vacío
	Address    string `json:"address"`
	Phone      string `json:"phone,omitempty"` // El campo omitempty indica que el campo puede ser omitido si está vacío
	Email      string `json:"email"`
	Web        string `json:"web,omitempty"`
	City       string `json:"city"`
	CategoryID int    `json:"category_id,omitempty"`
	Company    string `json:"company,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"` // Asume que este campo es manejado automáticamente por la base de datos
	UpdatedAt  string `json:"updated_at,omitempty"` // Asume que este campo es manejado automáticamente por la base de datos
}

// ContactWithProviders es una estructura extendida de Contact para incluir los ProviderIDs asociados
type ContactWithProviders struct {
	Contact           // Incorporación anónima de la estructura Contact
	ProviderIDs []int `json:"provider_ids"` // Slice de IDs de provideres
}

// ProviderContact representa la relación muchos a muchos entre Contactos y Provideres
type ProviderContact struct {
	ProviderID int `json:"provider_id"`
	ContactID  int `json:"contact_id"`
}
type Category struct {
	ID         int            `json:"id"`
	Name       string         `json:"name"`
	Fields     sql.NullString `json:"-"`                // No se serializará directamente
	FieldsJSON string         `json:"fields,omitempty"` // Omite si está vacío
	Parent     int            `json:"parent"`
}

// MarshalJSON personaliza la serialización de Category a JSON.
func (c Category) MarshalJSON() ([]byte, error) {
	var fields []string // Utiliza el tipo adecuado según el contenido de `fields`

	// Solo intenta deserializar si Fields.Valid es true y Fields.String no está vacío
	if c.Fields.Valid && c.Fields.String != "" {
		// Deserializa la cadena en un arreglo de strings
		err := json.Unmarshal([]byte(c.Fields.String), &fields)
		if err != nil {
			// Maneja el error de alguna manera, por ejemplo, puedes decidir loguearlo y continuar con un arreglo vacío
			fields = nil
		}
	}

	// Crea una instancia de CategoryJSON para la serialización,
	// usando los valores transformados según sea necesario.
	categoryJSON := CategoryJSON{
		ID:     c.ID,
		Name:   c.Name,
		Fields: fields, // Ahora es un arreglo, no una cadena
		Parent: c.Parent,
	}

	return json.Marshal(categoryJSON)
}

// CategoryJSON es una estructura intermedia para la serialización JSON.
type CategoryJSON struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Fields []string `json:"fields,omitempty"` // Omite si está vacío
	Parent int      `json:"parent"`
}

type Log struct {
	ID        int    `json:"id"`
	Type      string `json:"type"`
	OldValue  string `json:"old_value"`
	NewValue  string `json:"new_value"`
	UserID    int    `json:"user_id"`
	Username  string `json:"username,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type Location struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Lat     float64 `json:"lat,omitempty"` // Usa float64 para coordenadas
	Lng     float64 `json:"lng,omitempty"` // Usa float64 para coordenadas
	State   string  `json:"state,omitempty"`
	City    string  `json:"city"`
	Country string  `json:"country"`
}
type Pos struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Warehouse int    `json:"warehouse"`
}
type Project struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Code         string `json:"code,omitempty"`
	Description  string `json:"description,omitempty"`
	CategoryID   int    `json:"category_id,omitempty"`
	StatusID     int    `json:"status_id"`
	LocationID   int    `json:"location_id,omitempty"`
	AuthorID     int    `json:"author_id"`
	ClientID     int    `json:"client_id"`
	ClientName   string `json:"client_name,omitempty"`
	CategoryName string `json:"category_name,omitempty"`
	StatusName   string `json:"status_name,omitempty"`
	LocationName string `json:"location_name,omitempty"`
	AuthorName   string `json:"author_name,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"` // Asume que este campo es manejado automáticamente por la base de datos
	UpdatedAt    string `json:"updated_at,omitempty"` // Asume que este campo es manejado automáticamente por la base de datos
}

type Sale struct {
	ID        int            `json:"id"`
	Draft     string         `json:"draft"`
	POS       int            `json:"pos"`
	Items     string         `json:"items"`
	Gender    string         `json:"gender"`
	Age       string         `json:"age"`
	Payment   string         `json:"payment"`
	Total     string         `json:"total"`
	Note      sql.NullString `json:"note,omitempty"`
	Tax       sql.NullString `json:"tax,omitempty"`
	DNI       sql.NullString `json:"dni,omitempty"`
	Email     sql.NullString `json:"email,omitempty"`
	Phone     sql.NullString `json:"phone,omitempty"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

type SaleData struct {
	ID        int    `json:"id"`
	POS       int    `json:"pos"`
	Items     string `json:"items"`
	Payment   string `json:"payment"`
	Total     string `json:"total"`
	CreatedAt string `json:"created_at"`
}

type Setting struct {
	ID          int    `json:"id"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

type WhatsAppComponent struct {
	Type     string      `json:"type"`
	Document interface{} `json:"document"`
}

type WhatsAppMessage struct {
	MessagingProduct string              `json:"messaging_product"`
	To               string              `json:"to"`
	Type             string              `json:"type"`
	Template         WhatsAppTemplate    `json:"template"`
	Components       []WhatsAppComponent `json:"components"`
}

type WhatsAppTemplate struct {
	Name       string              `json:"name"`
	Language   WhatsAppLanguage    `json:"language"`
	Components []WhatsAppComponent `json:"components"`
}

type WhatsAppLanguage struct {
	Code string `json:"code"`
}
type SettingVal struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// Estructura para almacenar las reclamaciones (claims) del token JWT
type Claims struct {
	UserID uint `json:"user_id"`
	jwt.StandardClaims
}
