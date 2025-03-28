package main

import (
	"brotecolectivo/database"
	"brotecolectivo/handlers"
	"brotecolectivo/utils"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"gopkg.in/ini.v1"

	_ "github.com/go-sql-driver/mysql"
)

var totalRequests int
var uptime time.Time
var dataBase *database.DatabaseStruct
var jwtKey []byte

func main() {
	initConfig()
	defer dataBase.Close()
	var port string
	flag.StringVar(&port, "port", "3001", "Define el puerto en el que el servidor debería escuchar")
	flag.Parse()
	authHandler := handlers.NewAuthHandler(dataBase)

	r := InitRoutes(authHandler)

	uptime = time.Now()

	// Inicia el servidor en el puerto especificado
	log.Printf("Servidor corriendo en el puerto %s\n", port)
	http.ListenAndServe(fmt.Sprintf(":%s", port), r)
}

func initConfig() {
	var err error

	// Cargar el archivo de configuración
	cfg, err := ini.Load("data.conf")
	if err != nil {
		log.Fatal("Error al cargar el archivo de configuración: ", err)
	}

	// Leer las propiedades de la sección "database"
	dataSection := cfg.Section("keys")
	jwtKey = []byte(dataSection.Key("JWT_KEY").String())

	// Sincronizar la clave JWT con el paquete utils
	utils.SetJwtKey(jwtKey)

	dbSection := cfg.Section("database")

	// Inicializar la conexión a la base de datos
	dataBase, err = database.NewDatabase(
		dbSection.Key("DB_USER").String(),
		dbSection.Key("DB_PASS").String(),
		dbSection.Key("DB_NAME").String(),
		dbSection.Key("DB_HOST").String(),
	)
	if err != nil {
		log.Fatal("Error al conectar con la base de datos: ", err)
	}
}
