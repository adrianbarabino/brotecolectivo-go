# 🚀 Brote Colectivo API - Backend

API REST desarrollada en Go (Golang) para la plataforma cultural **Brote Colectivo**. Este backend proporciona todos los servicios necesarios para gestionar artistas, eventos, noticias, espacios culturales y el sistema de colaboraciones de la plataforma.

---

## 🧰 Tecnologías utilizadas

- [Go (Golang)](https://golang.org/) - Lenguaje de programación
- [Chi Router](https://github.com/go-chi/chi) - Router HTTP minimalista
- [MySQL](https://www.mysql.com/) - Base de datos relacional
- [JWT](https://github.com/golang-jwt/jwt) - Autenticación basada en tokens
- [AWS SDK](https://github.com/aws/aws-sdk-go) - Para almacenamiento de archivos en DigitalOcean Spaces
- [INI](https://github.com/go-ini/ini) - Manejo de archivos de configuración

---

## 📋 Requisitos previos

- Go 1.16 o superior
- MySQL 8.0 o superior
- Cuenta en DigitalOcean Spaces (o compatible con S3) para almacenamiento de imágenes

---

## 🚀 Instalación y configuración

### 1. Clonar el repositorio

```bash
git clone https://github.com/adrianbarabino/brotecolectivo-go.git
cd brotecolectivo-go
```

### 2. Configurar el archivo de datos

Crear un archivo `data.conf` en la raíz del proyecto con la siguiente estructura:

```ini
[database]
user = usuario_mysql
password = contraseña_mysql
host = localhost
name = brotecolectivo_portal
port = 3306

[keys]
JWT_KEY = tu_clave_secreta_jwt
whatsapp_number = tu_numero_whatsapp
whatsapp_token = tu_token_whatsapp

[spaces]
key = tu_access_key
secret = tu_secret_key
endpoint = sfo3.digitaloceanspaces.com
bucket = brotecolectivo
region = sfo3

[security]
approval_secret = tu_clave_secreta_para_aprobaciones
```

### 3. Instalar dependencias y compilar

```bash
go mod download
go build -o brotecolectivo-api
```

### 4. Ejecutar el servidor

```bash
./brotecolectivo-api
```

Por defecto, el servidor se ejecuta en el puerto 3001. Para cambiar el puerto:

```bash
./brotecolectivo-api -port=8080
```

---

## 📁 Estructura del proyecto

```
brotecolectivo-go/
│
├── database/           # Capa de acceso a datos
├── handlers/           # Manejadores de rutas HTTP
│   ├── bands.go        # Gestión de artistas
│   ├── events.go       # Gestión de eventos
│   ├── news.go         # Gestión de noticias
│   ├── submissions.go  # Sistema de colaboraciones
│   ├── venues.go       # Espacios culturales
│   └── ...
├── models/             # Definición de modelos de datos
├── utils/              # Utilidades y helpers
├── main.go             # Punto de entrada
├── routes.go           # Definición de rutas
└── data.conf           # Configuración (no incluido en repo)
```

---

## 🔌 API Endpoints

### Autenticación

- `POST /login` - Iniciar sesión con email y contraseña
- `POST /login/google` - Iniciar sesión con Google
- `GET /me` - Obtener información del usuario actual

### Artistas

- `GET /bands` - Listar todos los artistas
- `GET /bands/{id}` - Obtener un artista por ID
- `GET /bands/slug/{slug}` - Obtener un artista por slug
- `POST /admin/bands` - Crear un nuevo artista (requiere autenticación)
- `PUT /admin/bands/{id}` - Actualizar un artista (requiere autenticación)
- `DELETE /admin/bands/{id}` - Eliminar un artista (requiere autenticación)

### Eventos

- `GET /events` - Listar todos los eventos
- `GET /events/{id}` - Obtener un evento por ID
- `GET /events/slug/{slug}` - Obtener un evento por slug
- `POST /admin/events` - Crear un nuevo evento (requiere autenticación)
- `PUT /admin/events/{id}` - Actualizar un evento (requiere autenticación)
- `DELETE /admin/events/{id}` - Eliminar un evento (requiere autenticación)

### Noticias

- `GET /news` - Listar todas las noticias
- `GET /news/{id}` - Obtener una noticia por ID
- `GET /news/slug/{slug}` - Obtener una noticia por slug
- `POST /admin/news` - Crear una nueva noticia (requiere autenticación)
- `PUT /admin/news/{id}` - Actualizar una noticia (requiere autenticación)
- `DELETE /admin/news/{id}` - Eliminar una noticia (requiere autenticación)

### Espacios Culturales

- `GET /venues` - Listar todos los espacios culturales
- `GET /venues/{id}` - Obtener un espacio cultural por ID
- `GET /venues/slug/{slug}` - Obtener un espacio cultural por slug
- `POST /admin/venues` - Crear un nuevo espacio cultural (requiere autenticación)
- `PUT /admin/venues/{id}` - Actualizar un espacio cultural (requiere autenticación)
- `DELETE /admin/venues/{id}` - Eliminar un espacio cultural (requiere autenticación)

### Sistema de Colaboraciones

- `GET /admin/submissions` - Listar todas las colaboraciones (requiere autenticación)
- `GET /admin/submissions/{id}` - Obtener una colaboración por ID (requiere autenticación)
- `POST /submissions` - Crear una nueva colaboración
- `POST /admin/submissions/{id}/approve` - Aprobar una colaboración (requiere autenticación)
- `GET /direct-approve/{id}` - Aprobación directa vía enlace (requiere token)

---

## 🔐 Sistema de aprobación directa

El sistema incluye un mecanismo de aprobación directa para colaboraciones mediante enlaces que pueden ser enviados por WhatsApp u otros medios. Estos enlaces contienen un token seguro generado con HMAC-SHA256 que permite a los administradores aprobar contenido desde dispositivos móviles sin necesidad de iniciar sesión en el panel de administración.

### Flujo de aprobación:

1. Un usuario envía una colaboración (artista, evento, etc.)
2. El sistema envía una notificación por WhatsApp a los administradores
3. El administrador puede aprobar directamente haciendo clic en el enlace
4. Tras la aprobación, se muestra una página de confirmación amigable con los detalles del contenido aprobado

---

## 📱 Integración con WhatsApp

La API incluye integración con la API de WhatsApp Business para enviar notificaciones sobre nuevas colaboraciones a los administradores. Esto permite una gestión más ágil del contenido y facilita la moderación desde dispositivos móviles.

---

## 🌐 Despliegue

### Despliegue en servidor Linux

1. Compilar para Linux:
   ```bash
   GOOS=linux GOARCH=amd64 go build -o brotecolectivo-api
   ```

2. Transferir el binario y el archivo de configuración al servidor

3. Configurar como servicio systemd:
   ```ini
   [Unit]
   Description=Brote Colectivo API
   After=network.target

   [Service]
   User=brotecolectivo
   WorkingDirectory=/home/brotecolectivo/api
   ExecStart=/home/brotecolectivo/api/brotecolectivo-api
   Restart=always

   [Install]
   WantedBy=multi-user.target
   ```

4. Habilitar y iniciar el servicio:
   ```bash
   sudo systemctl enable brotecolectivo-api
   sudo systemctl start brotecolectivo-api
   ```

### Configuración con Nginx

Para exponer la API en un subdominio (api.brotecolectivo.com) usando Nginx:

```nginx
server {
    listen 80;
    server_name api.brotecolectivo.com;

    location / {
        proxy_pass http://localhost:3001;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 👨‍💻 Colaboración

¿Querés contribuir al proyecto? ¡Genial! Seguí estos pasos:

1. Hacé un fork del repositorio
2. Creá una nueva rama (`git checkout -b feature/nueva-funcionalidad`)
3. Commiteá tus cambios (`git commit -m 'Agrega nueva funcionalidad'`)
4. Pusheá a la rama (`git push origin feature/nueva-funcionalidad`)
5. Abrí un Pull Request

---

Desarrollado con ❤️ por el equipo de Brote Colectivo