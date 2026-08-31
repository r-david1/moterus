// Comando verificador-auditoria recorre la bitácora de auditoría y valida
// la cadena de hashes (ADR 0005) invocando la función de BD
// verificar_cadena_auditoria, que es la única fuente de verdad sobre la
// integridad de la cadena: recalcula el hash esperado de cada fila con la
// misma serialización canónica que usó el trigger de inserción y lo
// compara contra el hash_actual almacenado.
//
// Uso:
//
//	go run ./cmd/verificador-auditoria [secuencia_desde]
//
// Sale con código 0 y sin salida en stdout si la cadena es íntegra desde
// secuencia_desde (por defecto 1). Sale con código 1 e imprime cada
// discrepancia encontrada en caso contrario — pensado para correr como job
// periódico y alimentar una alerta si el exit code no es 0.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	desde := int64(1)
	if len(os.Args) > 1 {
		v, err := strconv.ParseInt(os.Args[1], 10, 64)
		if err != nil {
			log.Fatalf("verificador-auditoria: secuencia_desde inválida %q: %v", os.Args[1], err)
		}
		desde = v
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("verificador-auditoria: falta la variable de entorno DATABASE_URL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("verificador-auditoria: no se pudo conectar: %v", err)
	}
	defer pool.Close()

	filas, err := pool.Query(ctx,
		"SELECT secuencia, problema, valor_esperado, valor_encontrado FROM verificar_cadena_auditoria($1)",
		desde,
	)
	if err != nil {
		log.Fatalf("verificador-auditoria: fallo al verificar la cadena: %v", err)
	}
	defer filas.Close()

	huboProblemas := false
	for filas.Next() {
		var secuencia int64
		var problema, esperado, encontrado string
		if err := filas.Scan(&secuencia, &problema, &esperado, &encontrado); err != nil {
			log.Fatalf("verificador-auditoria: fallo al leer resultado: %v", err)
		}
		huboProblemas = true
		fmt.Printf("secuencia=%d problema=%q esperado=%q encontrado=%q\n", secuencia, problema, esperado, encontrado)
	}
	if err := filas.Err(); err != nil {
		log.Fatalf("verificador-auditoria: error de cursor: %v", err)
	}

	if huboProblemas {
		log.Println("verificador-auditoria: la cadena de hashes tiene discrepancias — ver detalle arriba")
		os.Exit(1)
	}
	log.Printf("verificador-auditoria: cadena íntegra desde secuencia=%d\n", desde)
}
