package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// OrganizacionesCasoDeUso implementa puertos.GestorDeOrganizaciones: Crear
// (§3.1 del diseño, este archivo), Actualizar (§5, actualizar_organizacion.go)
// y CambiarEstado (§3.4, cambiar_estado_organizacion.go). Los tres comparten
// dependencias y el mismo criterio de orquestación (autorizar salvo Crear,
// abrir UnidadDeTrabajo, mutar, auditar, publicar fuera de la transacción).
type OrganizacionesCasoDeUso struct {
	organizaciones puertos.RepositorioOrganizaciones
	membresias     puertos.RepositorioMembresias
	autorizador    puertos.VerificadorDeAutorizacion
	sujetos        puertos.VerificadorDeSujetos
	confianza      puertos.EvaluadorConfianza
	auditoria      puertos.RegistroAuditoria
	eventos        puertos.PublicadorEventos
	reloj          puertos.Reloj
	ids            puertos.GeneradorIDs
	uow            puertos.UnidadDeTrabajo
	politica       dominio.PoliticaOrganizacion
}

var _ puertos.GestorDeOrganizaciones = (*OrganizacionesCasoDeUso)(nil)

// NuevoOrganizacionesCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoOrganizacionesCasoDeUso(
	organizaciones puertos.RepositorioOrganizaciones,
	membresias puertos.RepositorioMembresias,
	autorizador puertos.VerificadorDeAutorizacion,
	sujetos puertos.VerificadorDeSujetos,
	confianza puertos.EvaluadorConfianza,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	ids puertos.GeneradorIDs,
	uow puertos.UnidadDeTrabajo,
	politica dominio.PoliticaOrganizacion,
) *OrganizacionesCasoDeUso {
	return &OrganizacionesCasoDeUso{
		organizaciones: organizaciones,
		membresias:     membresias,
		autorizador:    autorizador,
		sujetos:        sujetos,
		confianza:      confianza,
		auditoria:      auditoria,
		eventos:        eventos,
		reloj:          reloj,
		ids:            ids,
		uow:            uow,
		politica:       politica,
	}
}

// Crear ejecuta el flujo normativo de §3.1 del diseño. Es el único caso de
// uso del contexto que no requiere autorización previa de Tenencia:
// cualquier sujeto autenticado puede fundar una organización, porque
// todavía no existe ninguna respecto de la cual autorizarlo.
func (c *OrganizacionesCasoDeUso) Crear(ctx context.Context, cmd puertos.ComandoCrearOrganizacion) (puertos.VistaOrganizacion, error) {
	idSujeto, err := dominio.IDUsuarioDesde(cmd.IDSujeto)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	decision, err := c.confianza.Evaluar(ctx, puertos.SolicitudEvaluacion{
		Accion:      "crear_organizacion",
		ClaveCuenta: claveCuentaUsuario(idSujeto),
		Origen:      cmd.Origen,
	})
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	if !decision.Permitido {
		return puertos.VistaOrganizacion{}, &dominio.ErrAccesoDenegadoPorConfianza{Motivo: decision.Motivo, ReintentarEn: decision.ReintentarEn}
	}

	sujeto, err := c.sujetos.EsElegible(ctx, idSujeto.String())
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	if !sujeto.Existe || !sujeto.Activo {
		return puertos.VistaOrganizacion{}, &dominio.ErrSujetoNoElegible{}
	}

	total, err := c.membresias.ContarOrganizacionesPropiasDeUsuario(ctx, idSujeto)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	if total >= c.politica.MaximoOrganizacionesPorUsuario() {
		return puertos.VistaOrganizacion{}, &dominio.ErrLimiteOrganizacionesExcedido{Limite: c.politica.MaximoOrganizacionesPorUsuario()}
	}

	alias, err := dominio.NuevoAlias(cmd.Alias)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	nombre, err := dominio.NuevoNombre(cmd.Nombre)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	idOrganizacion, err := c.ids.NuevoIDOrganizacion()
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	ahora := c.reloj.Ahora()
	org, err := dominio.CrearOrganizacion(idOrganizacion, alias, nombre, idSujeto, ahora)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	idMembresia, err := c.ids.NuevoIDMembresia()
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	fundadora, err := dominio.FundarMembresia(idMembresia, idOrganizacion, idSujeto, ahora)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	// INV-TEN-03: Organizacion y Membresia (fundacional) se persisten y
	// auditan en la MISMA UnidadDeTrabajo, sin excepciones.
	eventosOrg := org.EventosPendientes()
	eventosMembresia := fundadora.EventosPendientes()
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.organizaciones.Guardar(ctx, org); err != nil {
			return err
		}
		if err := c.membresias.Guardar(ctx, fundadora); err != nil {
			return err
		}
		for _, e := range eventosOrg {
			if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
				return err
			}
		}
		for _, e := range eventosMembresia {
			if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	todos := append(append([]dominio.EventoDominio{}, eventosOrg...), eventosMembresia...)
	if errPub := c.eventos.Publicar(ctx, todos...); errPub != nil {
		slog.WarnContext(ctx, "crear organización: fallo al publicar eventos",
			"error", errPub, "organizacion_id", idOrganizacion.String())
	}
	if errReg := c.confianza.RegistrarResultado(ctx, puertos.ResultadoIntento{
		Accion:      "crear_organizacion",
		ClaveCuenta: claveCuentaUsuario(idSujeto),
		Origen:      cmd.Origen,
		Exitoso:     true,
		IDUsuario:   idSujeto.String(),
	}); errReg != nil {
		slog.WarnContext(ctx, "crear organización: fallo al registrar el resultado en Confianza",
			"error", errReg, "organizacion_id", idOrganizacion.String())
	}

	return vistaOrganizacionDesde(org, 1), nil
}
