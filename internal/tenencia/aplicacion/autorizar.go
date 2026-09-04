package aplicacion

import (
	"context"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// AutorizarCasoDeUso implementa puertos.VerificadorDeAutorizacion: el camino
// caliente del sistema (§3.7 del diseño), consumido por el middleware de
// autorización de CUALQUIER contexto y, dentro de este mismo paquete, por
// los demás casos de uso que necesitan re-autorizar una acción específica
// (§3.2, §3.3, §3.4, §3.5, §3.6).
//
// NOTA sobre el presupuesto de "una consulta indexada" del diseño: el
// puerto RepositorioMembresias.BuscarVigente devuelve únicamente
// *dominio.Membresia, sin el estado de la organización, aunque §3.7 paso 2
// describe la consulta con un JOIN a organizaciones. dominio.Autorizar
// exige el EstadoOrganizacion como parámetro explícito (función pura, sin
// E/S), así que este caso de uso necesita una SEGUNDA lectura
// (RepositorioOrganizaciones.BuscarPorID) para obtenerlo. Se minimiza el
// costo en el camino de denegación más frecuente (sin membresía o membresía
// suspendida) evaluando esos dos pasos ANTES de tocar
// RepositorioOrganizaciones, de modo que solo el camino "membresía activa"
// paga la segunda consulta. Este es un hueco del puerto de salida
// (puertos/salida.go), no de este caso de uso: para cerrarlo de verdad,
// RepositorioMembresias.BuscarVigente debería devolver también el estado de
// la organización (p. ej. un tipo compuesto), tal como ya lo hace
// ConsultorDeMembresias.RolEnOrganizacion vía VistaRolEfectivo. Reportado en
// el resumen de la tarea; no se modifica puertos/salida.go aquí.
type AutorizarCasoDeUso struct {
	organizaciones puertos.RepositorioOrganizaciones
	membresias     puertos.RepositorioMembresias
	auditoria      puertos.RegistroAuditoria
	reloj          puertos.Reloj
}

var _ puertos.VerificadorDeAutorizacion = (*AutorizarCasoDeUso)(nil)

// NuevoAutorizarCasoDeUso construye el caso de uso con sus dependencias
// inyectadas por puerto.
func NuevoAutorizarCasoDeUso(
	organizaciones puertos.RepositorioOrganizaciones,
	membresias puertos.RepositorioMembresias,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
) *AutorizarCasoDeUso {
	return &AutorizarCasoDeUso{
		organizaciones: organizaciones,
		membresias:     membresias,
		auditoria:      auditoria,
		reloj:          reloj,
	}
}

// Autorizar ejecuta el flujo normativo de §3.7 del diseño:
//  1. Construir IDUsuario, IDOrganizacion y Permiso; un permiso fuera del
//     catálogo cerrado es un bug del llamador (ErrPermisoDesconocido), no
//     una denegación.
//  2. RepositorioMembresias.BuscarVigente: la lectura del camino caliente.
//  3. Solo si hay membresía ACTIVA se paga la segunda lectura
//     (RepositorioOrganizaciones.BuscarPorID) para conocer el estado de la
//     organización; en los dos motivos de denegación más frecuentes
//     (sin_membresia, membresia_suspendida) el estado de la organización es
//     irrelevante para dominio.Autorizar y no se consulta.
//  4. dominio.Autorizar(estadoOrg, membresia, permiso): función pura.
//  5. Permitido: se devuelve SIN auditar (INV-TEN-25). Denegado: se audita
//     AutorizacionDenegada y se devuelve la decisión con su motivo.
func (c *AutorizarCasoDeUso) Autorizar(ctx context.Context, q puertos.ConsultaAutorizacion) (puertos.Autorizacion, error) {
	idUsuario, err := dominio.IDUsuarioDesde(q.IDUsuario)
	if err != nil {
		return puertos.Autorizacion{}, err
	}
	idOrganizacion, err := dominio.IDOrganizacionDesde(q.IDOrganizacion)
	if err != nil {
		return puertos.Autorizacion{}, err
	}
	permiso, err := dominio.PermisoDesde(q.Permiso)
	if err != nil {
		return puertos.Autorizacion{}, err
	}

	membresia, err := c.membresias.BuscarVigente(ctx, idUsuario, idOrganizacion)
	if err != nil {
		return puertos.Autorizacion{}, err
	}

	var estadoOrg dominio.EstadoOrganizacion
	if membresia != nil && membresia.Estado().EsIgual(dominio.EstadoMembresiaActiva) {
		org, err := c.organizaciones.BuscarPorID(ctx, idOrganizacion)
		if err != nil {
			return puertos.Autorizacion{}, err
		}
		if org != nil {
			estadoOrg = org.Estado()
		}
		// org == nil: violación de integridad referencial (no debería
		// ocurrir); estadoOrg queda en su valor cero, que dominio.Autorizar
		// trata como "no activa" -> denegado(organizacion_no_operativa).
	}

	decision := dominio.Autorizar(estadoOrg, membresia, permiso)
	if decision.Permitido() {
		return autorizacionDesde(idOrganizacion, decision), nil
	}

	motivo, _ := decision.Motivo()
	rolActual := ""
	if rol, ok := decision.Rol(); ok {
		rolActual = rol.Valor()
	}
	evento := dominio.NuevoAutorizacionDenegada(idUsuario, idOrganizacion, permiso, motivo, rolActual, c.reloj.Ahora())
	if err := c.auditoria.Registrar(ctx, evento, idOrganizacion, q.Origen); err != nil {
		return puertos.Autorizacion{}, err
	}
	return autorizacionDesde(idOrganizacion, decision), nil
}

func autorizacionDesde(idOrganizacion dominio.IDOrganizacion, d dominio.DecisionAutorizacion) puertos.Autorizacion {
	rolValor := ""
	if rol, ok := d.Rol(); ok {
		rolValor = rol.Valor()
	}
	motivoValor := ""
	if motivo, ok := d.Motivo(); ok {
		motivoValor = motivo.Valor()
	}
	return puertos.Autorizacion{
		Permitido:      d.Permitido(),
		IDOrganizacion: idOrganizacion.String(),
		Rol:            rolValor,
		Motivo:         motivoValor,
	}
}
