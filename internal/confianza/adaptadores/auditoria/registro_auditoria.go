// Package auditoria implementa confianza/puertos.RegistroAuditoria
// insertando en la tabla auditoria del contexto Auditoría (ADR 0005) —
// mismo mecanismo ya construido y reutilizado por Identidad, Acceso y
// Tenencia: misma tabla, mismo trigger auditoria_asignar_cadena, mismo
// catálogo cerrado auditoria_acciones (§9 del diseño
// docs/design/colas-virtuales.md: "no se inventa ningún mecanismo nuevo").
//
// A diferencia del ACL homónimo de Tenencia, la firma de Registrar NO lleva
// IDOrganizacion (mismo criterio que Identidad y Acceso, y el comentario de
// confianza/puertos.RegistroAuditoria: "sin IDOrganizacion propio, porque
// una sala de alcance sistema no tiene una organización que poblar en
// auditoria.organizacion_id"): organizacion_id viaja siempre NULL desde
// este ACL. Confianza es la primera bitácora propia de este contexto (§9
// del diseño).
package auditoria

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// ejecutorSQL es el subconjunto de pgxpool.Pool / pgx.Tx que este ACL
// necesita. Se declara localmente (no se importa sqlc.DBTX) porque este
// paquete no usa código generado por sqlc: no hay queries de auditoria en
// db/consultas todavía (mismo criterio que identidad/adaptadores/auditoria
// y acceso/adaptadores/auditoria).
type ejecutorSQL interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

const insertarAuditoria = `
INSERT INTO auditoria (
    usuario_id, organizacion_id, accion, recurso, recurso_id, resultado,
    ip_origen, huella_dispositivo, id_solicitud, detalles, ocurrido_en
) VALUES (
    $1, NULL, $2, $3, $4, $5,
    $6, $7, $8, $9, $10
)`

// RegistroAuditoria implementa puertos.RegistroAuditoria. organizacion_id
// siempre viaja NULL: ninguno de los tres eventos de dominio de Confianza
// (SalaDeEsperaAbierta, RitmoDeAdmisionCambiado, SalaDeEsperaCerrada) lleva
// un IDOrganizacion propio en su forma de evento (la sala puede ser de
// alcance sistema, sin organización dueña alguna), y el puerto de Confianza
// no lo transporta (a diferencia de tenencia/puertos.RegistroAuditoria).
type RegistroAuditoria struct {
	pool *pgxpool.Pool
}

var _ puertos.RegistroAuditoria = (*RegistroAuditoria)(nil)

// NuevoRegistroAuditoria construye el adaptador sobre un pool pgx ya
// inicializado.
func NuevoRegistroAuditoria(pool *pgxpool.Pool) *RegistroAuditoria {
	return &RegistroAuditoria{pool: pool}
}

// Registrar inserta la fila de auditoría correspondiente a e. Participa en
// la transacción publicada por UnidadDeTrabajo.Ejecutar (ADR 0005: negocio
// + auditoría en la misma transacción, INV-COLA-11: las tres mutaciones de
// ciclo de vida de una sala se auditan siempre en la misma unidad de
// trabajo que la escritura); si no hay transacción en ctx, inserta
// directamente contra el pool.
//
// version_hash, secuencia, hash_anterior y hash_actual NO se envían: los
// asigna el trigger auditoria_asignar_cadena en la base de datos.
func (r *RegistroAuditoria) Registrar(ctx context.Context, e dominio.EventoDominio, origen dominio.OrigenSolicitud) error {
	fila, reconocido := mapearEvento(e)
	if !reconocido {
		return fmt.Errorf("auditoria: evento de dominio no reconocido en el catálogo cerrado: %T", e)
	}

	detallesJSON, err := json.Marshal(fila.detalles)
	if err != nil {
		return fmt.Errorf("auditoria: no se pudo serializar detalles: %w", err)
	}

	ejecutor := r.ejecutor(ctx)

	_, err = ejecutor.Exec(ctx, insertarAuditoria,
		valorONulo(fila.usuarioID),
		fila.accion,
		fila.recurso,
		valorONulo(fila.recursoID),
		fila.resultado,
		valorIPONulo(origen),
		valorONulo(origen.HuellaDispositivo()),
		valorONulo(origen.IDSolicitud()),
		string(detallesJSON),
		e.OcurridoEn(),
	)
	if err != nil {
		return fmt.Errorf("auditoria: no se pudo insertar el evento %s: %w", e.NombreEvento(), err)
	}
	return nil
}

func (r *RegistroAuditoria) ejecutor(ctx context.Context) ejecutorSQL {
	if tx, ok := bd.TxDesdeContexto(ctx); ok {
		return tx
	}
	return r.pool
}

// valorONulo devuelve nil (NULL en SQL) para una cadena vacía, en vez de
// insertar "" — usuario_id/recurso_id/huella_dispositivo/id_solicitud son
// columnas nullable donde "" y "sin dato" no deben confundirse.
func valorONulo(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// valorIPONulo extrae la IP de origen como texto (la columna ip_origen es
// INET; pgx castea un string válido automáticamente), o nil si la
// solicitud no traía IP (p. ej. una sala de alcance sistema operada fuera
// de la API, §7.2 del diseño).
func valorIPONulo(origen dominio.OrigenSolicitud) any {
	if origen.IP().EsVacia() {
		return nil
	}
	return origen.IP().String()
}
