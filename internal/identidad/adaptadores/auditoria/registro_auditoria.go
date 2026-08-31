// Este archivo implementa identidad/puertos.RegistroAuditoria insertando en
// la tabla auditoria del contexto Auditoría (ADR 0005). No existe todavía
// un adaptador propio del contexto Auditoría con queries sqlc (db/consultas
// solo tiene identidad.sql), así que este ACL inserta con SQL directo vía
// pgx; si en el futuro Auditoría expone su propio paquete de
// aplicación/puertos, este archivo es el único que hay que tocar (la capa
// anticorrupción vive en el consumidor — sección 5, punto 3 del diseño de
// Identidad).
package auditoria

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// ejecutorSQL es el subconjunto de pgxpool.Pool / pgx.Tx que este ACL
// necesita. Se declara localmente (en vez de importar sqlc.DBTX) porque
// este paquete no usa código generado por sqlc: no hay queries de auditoria
// en db/consultas todavía.
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
// siempre viaja NULL: el contexto Tenencia no existe todavía (ver
// db/migraciones/000002_crear_auditoria.up.sql, comentario final).
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
// la transacción publicada por UnidadDeTrabajo.Ejecutar cuando existe
// (ADR 0005: negocio + auditoría en la misma transacción), y si no hay
// transacción en ctx, inserta directamente contra el pool (p. ej. el
// registro de auditoría de un rechazo de Confianza antes de que exista
// UnidadDeTrabajo en el flujo, como en RegistrarUsuarioCasoDeUso paso 1).
//
// version_hash, secuencia, hash_anterior y hash_actual NO se envían: los
// asigna el trigger auditoria_asignar_cadena en la base de datos (sección 6
// del diseño; comentario de cabecera de 000002_crear_auditoria.up.sql). Si
// este adaptador los calculara, estaría reimplementando (y pudiendo
// desincronizar) la única fuente de verdad de la cadena.
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
// solicitud no traía IP (p. ej. una llamada interna del sistema).
func valorIPONulo(origen dominio.OrigenSolicitud) any {
	if origen.IP().EsVacia() {
		return nil
	}
	return origen.IP().String()
}
