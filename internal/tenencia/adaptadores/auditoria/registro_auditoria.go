// Package auditoria implementa tenencia/puertos.RegistroAuditoria
// insertando en la tabla auditoria del contexto Auditoría (ADR 0005) —
// mismo mecanismo ya construido y reutilizado por Identidad y Acceso: misma
// tabla, mismo trigger auditoria_asignar_cadena, mismo catálogo cerrado
// auditoria_acciones (§8 del diseño de Tenencia: "no se inventa ningún
// mecanismo nuevo").
//
// Diferencia respecto de los ACL homónimos de Identidad y Acceso: la firma
// de Registrar lleva IDOrganizacion, y este ACL es el primero en poblar
// auditoria.organizacion_id con un valor real (NULL desde la migración
// 000002, hasta ahora — INV-TEN-26).
package auditoria

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/r-david1/moterus/internal/plataforma/bd"
	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
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
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11
)`

// RegistroAuditoria implementa puertos.RegistroAuditoria.
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
// AutorizacionDenegada, que AutorizarCasoDeUso registra fuera de cualquier
// UnidadDeTrabujo — el camino caliente de autorización no abre
// transacciones de escritura de negocio).
//
// version_hash, secuencia, hash_anterior y hash_actual NO se envían: los
// asigna el trigger auditoria_asignar_cadena en la base de datos.
//
// org nunca debería llegar vacío (todo evento de Tenencia ocurre dentro de
// una organización, INV-TEN-26), pero si algún día lo hiciera, se persiste
// como NULL en vez de fallar la inserción: la bitácora es más importante
// que la completitud de un campo opcional.
func (r *RegistroAuditoria) Registrar(ctx context.Context, e dominio.EventoDominio, org dominio.IDOrganizacion, origen dominio.OrigenSolicitud) error {
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
		valorONulo(org.String()),
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
// traía IP.
func valorIPONulo(origen dominio.OrigenSolicitud) any {
	if origen.IP().EsVacia() {
		return nil
	}
	return origen.IP().String()
}
