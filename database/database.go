package database

import (
	"database/sql"
	"fmt"
	"log"
)

type DatabaseStruct struct {
	connection *sql.DB
}

func NewDatabase(dbUser, dbPass, dbName, dbHost string) (*DatabaseStruct, error) {

	// Construir la cadena de conexión
	connectionString := fmt.Sprintf("%s:%s@tcp(%s)/%s", dbUser, dbPass, dbHost, dbName)

	db, err := sql.Open("mysql", connectionString)

	// Cambia los detalles de conexión según tu configuración de MySQL
	if err != nil {
		log.Fatal(err)
	}

	err = db.Ping()

	return &DatabaseStruct{connection: db}, err
}

func (db *DatabaseStruct) Close() {
	db.connection.Close()
}
func (db *DatabaseStruct) Insert(prepare bool, query string, args ...interface{}) (int, error) {
	var result sql.Result
	var err error

	if prepare {
		stmt, err := db.connection.Prepare(query)
		if err != nil {
			return 0, fmt.Errorf("error al preparar statement: %w", err)
		}
		defer stmt.Close()
		result, err = stmt.Exec(args...)
	} else {
		result, err = db.connection.Exec(query, args...)
	}

	if err != nil {
		return 0, fmt.Errorf("error al ejecutar insert: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("error al obtener LastInsertId: %w", err)
	}

	if id == 0 {
		return 0, fmt.Errorf("insert ejecutado pero ID devuelto es 0 (probable fallo)")
	}

	return int(id), nil
}

func (db *DatabaseStruct) Update(prepare bool, query string, args ...interface{}) (int64, error) {
	if prepare {
		stmt, err := db.connection.Prepare(query)
		if err != nil {
			return 0, err
		}
		defer stmt.Close()
	}
	result, err := db.connection.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (db *DatabaseStruct) Delete(prepare bool, query string, args ...interface{}) (int64, error) {
	if prepare {
		stmt, err := db.connection.Prepare(query)
		if err != nil {
			return 0, err
		}
		defer stmt.Close()
	}
	result, err := db.connection.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

// el retorno rows requiere un defer rows.Close()
func (db *DatabaseStruct) Select(query string, args ...interface{}) (*sql.Rows, error) {
	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (db *DatabaseStruct) SelectRow(query string, args ...interface{}) (*sql.Row, error) {
	row := db.connection.QueryRow(query, args...)
	return row, nil
}

// Exec executes a query without returning any rows
func (db *DatabaseStruct) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.connection.Exec(query, args...)
}

// CheckArtistLinksTable verifica si la tabla artist_links existe y muestra su estructura
func (db *DatabaseStruct) CheckArtistLinksTable() {
	// Verificar si la tabla existe
	var tableName string
	row := db.connection.QueryRow("SHOW TABLES LIKE 'artist_links'")
	err := row.Scan(&tableName)

	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Println("DIAGNÓSTICO: La tabla artist_links NO EXISTE en la base de datos")
		} else {
			fmt.Printf("DIAGNÓSTICO: Error al verificar existencia de tabla: %v\n", err)
		}
		return
	}

	fmt.Println("DIAGNÓSTICO: La tabla artist_links EXISTE en la base de datos")

	// Mostrar la estructura de la tabla
	rows, err := db.connection.Query("DESCRIBE artist_links")
	if err != nil {
		fmt.Printf("DIAGNÓSTICO: Error al obtener estructura de tabla: %v\n", err)
		return
	}
	defer rows.Close()

	fmt.Println("DIAGNÓSTICO: Estructura de la tabla artist_links:")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("%-15s %-20s %-10s %-10s %-15s\n", "Campo", "Tipo", "Nulo", "Clave", "Predeterminado")
	fmt.Println("--------------------------------------------------")

	for rows.Next() {
		var field, fieldType, null, key, defaultVal, extra sql.NullString
		if err := rows.Scan(&field, &fieldType, &null, &key, &defaultVal, &extra); err != nil {
			fmt.Printf("DIAGNÓSTICO: Error al escanear estructura: %v\n", err)
			continue
		}

		nullStr := "NO"
		if null.Valid && null.String == "YES" {
			nullStr = "YES"
		}

		defaultValStr := "NULL"
		if defaultVal.Valid {
			defaultValStr = defaultVal.String
		}

		fmt.Printf("%-15s %-20s %-10s %-10s %-15s\n",
			field.String,
			fieldType.String,
			nullStr,
			key.String,
			defaultValStr)
	}

	// Verificar si hay registros en la tabla
	var count int
	countRow := db.connection.QueryRow("SELECT COUNT(*) FROM artist_links")
	if err := countRow.Scan(&count); err != nil {
		fmt.Printf("DIAGNÓSTICO: Error al contar registros: %v\n", err)
	} else {
		fmt.Printf("DIAGNÓSTICO: La tabla artist_links contiene %d registros\n", count)
	}
}
