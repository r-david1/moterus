// Comando migrador embebe golang-migrate para aplicar/revertir las
// migraciones de db/migraciones contra DATABASE_URL. Es infraestructura
// pura: no conoce el esquema ni las reglas de ningún bounded context.
//
// Uso:
//
//	go run ./cmd/migrador up
//	go run ./cmd/migrador down
//	go run ./cmd/migrador version
package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const rutaMigraciones = "file://db/migraciones"

func main() {
	if len(os.Args) < 2 {
		log.Fatal("uso: migrador <up|down|version>")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("migrador: falta la variable de entorno DATABASE_URL")
	}

	m, err := migrate.New(rutaMigraciones, dsn)
	if err != nil {
		log.Fatalf("migrador: no se pudo inicializar: %v", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	switch os.Args[1] {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	case "version":
		v, dirty, verErr := m.Version()
		if verErr != nil {
			log.Fatalf("migrador: %v", verErr)
		}
		fmt.Printf("version=%d dirty=%v\n", v, dirty)
		return
	default:
		log.Fatalf("migrador: comando desconocido %q", os.Args[1])
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("migrador: %v", err)
	}
	log.Println("migrador: completado")
}
