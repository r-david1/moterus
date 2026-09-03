// Package auditoria implementa acceso/puertos.RegistroAuditoria insertando
// en la tabla auditoria del contexto Auditoría (ADR 0005) — mismo
// mecanismo ya construido y reutilizado por Identidad
// (identidad/adaptadores/auditoria/registro_auditoria.go): misma tabla,
// mismo trigger auditoria_asignar_cadena, mismo catálogo cerrado
// auditoria_acciones (§8 del diseño de Acceso: "no se inventa ningún
// mecanismo nuevo"). No existe un paquete de aplicación propio del
// contexto Auditoría con queries sqlc reutilizables entre contextos (solo
// db/consultas/identidad.sql y db/consultas/acceso.sql, cada uno acotado a
// sus propias tablas — INV-ID-20/INV-ACC-20), así que este ACL, igual que
// el de Identidad, inserta con SQL directo vía pgx.
package auditoria

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
	"github.com/r-david1/moterus/internal/plataforma/bd"
)

// ejecutorSQL es el subconjunto de pgxpool.Pool / pgx.Tx que este ACL
// necesita.
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
// siempre viaja NULL: Tenencia no existe todavía (§6 del diseño de
// Acceso, misma nota que la migración 000006).
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
// la transacción publicada por UnidadDeTrabajo.Ejecutar cuando existe (ADR
// 0005: negocio + auditoría en la misma transacción); si no hay
// transacción en ctx, inserta directamente contra el pool (p. ej.
// RenovacionRechazada cuando el refresco es desconocido: no hay agregado
// que guardar, solo el intento que auditar).
//
// version_hash, secuencia, hash_anterior y hash_actual NO se envían: los
// asigna el trigger auditoria_asignar_cadena en la base de datos (§8 del
// diseño).
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

// valorONulo devuelve nil (NULL en SQL) para una cadena vacía.
func valorONulo(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// valorIPONulo extrae la IP de origen como texto, o nil si la solicitud no
// traía IP (p. ej. la revalidación de estado del sujeto en cada
// renovación, que es una llamada interna del sistema).
func valorIPONulo(origen dominio.OrigenSolicitud) any {
	if origen.IP().EsVacia() {
		return nil
	}
	return origen.IP().String()
}
