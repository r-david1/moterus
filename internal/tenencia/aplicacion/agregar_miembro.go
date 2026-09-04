package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// MembresiasCasoDeUso implementa puertos.GestorDeMembresias: Agregar (§3.3
// del diseño, este archivo), CambiarRol, Remover, Abandonar y
// TransferirPropiedad (§3.2, un archivo por operación). Los cuatro últimos
// comparten el esqueleto descrito una sola vez en §3.2: autorizar (salvo
// Abandonar), candado de fila de la organización
// (RepositorioOrganizaciones.CargarParaActualizar) ANTES de contar
// propietarios activos, comprobación de que la organización sigue
// operativa, resolución de la membresía objetivo, y paso del conteo de
// propietarios activos como parámetro al método de negocio correspondiente
// (nunca lo consulta el dominio, INV-TEN-27).
type MembresiasCasoDeUso struct {
	organizaciones puertos.RepositorioOrganizaciones
	membresias     puertos.RepositorioMembresias
	autorizador    puertos.VerificadorDeAutorizacion
	sujetos        puertos.VerificadorDeSujetos
	auditoria      puertos.RegistroAuditoria
	eventos        puertos.PublicadorEventos
	reloj          puertos.Reloj
	ids            puertos.GeneradorIDs
	uow            puertos.UnidadDeTrabajo
	politica       dominio.PoliticaOrganizacion
}

var _ puertos.GestorDeMembresias = (*MembresiasCasoDeUso)(nil)

// NuevoMembresiasCasoDeUso construye el caso de uso con sus dependencias
// inyectadas por puerto.
func NuevoMembresiasCasoDeUso(
	organizaciones puertos.RepositorioOrganizaciones,
	membresias puertos.RepositorioMembresias,
	autorizador puertos.VerificadorDeAutorizacion,
	sujetos puertos.VerificadorDeSujetos,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	ids puertos.GeneradorIDs,
	uow puertos.UnidadDeTrabajo,
	politica dominio.PoliticaOrganizacion,
) *MembresiasCasoDeUso {
	return &MembresiasCasoDeUso{
		organizaciones: organizaciones,
		membresias:     membresias,
		autorizador:    autorizador,
		sujetos:        sujetos,
		auditoria:      auditoria,
		eventos:        eventos,
		reloj:          reloj,
		ids:            ids,
		uow:            uow,
		politica:       politica,
	}
}

// Agregar implementa AgregarMiembro (§3.3 del diseño): alta directa, sin
// invitación. Autoriza con miembro.invitar — el catálogo cerrado de 8
// permisos no distingue "invitar por correo" de "dar de alta
// directamente"; ambas son formas de incorporar un miembro y comparten el
// mismo riesgo de escalada, por lo que comparten el mismo permiso y la
// misma regla de dominancia sobre el rol otorgado (INV-TEN-20). Valida
// elegibilidad vía VerificadorDeSujetos ANTES de crear la membresía
// (INV-TEN-07): nunca confía en que el IDUsuario recibido existe.
func (c *MembresiasCasoDeUso) Agregar(ctx context.Context, cmd puertos.ComandoAgregarMiembro) (puertos.VistaMiembro, error) {
	idOrganizacion, err := dominio.IDOrganizacionDesde(cmd.IDOrganizacion)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	idObjetivo, err := dominio.IDUsuarioDesde(cmd.IDUsuario)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	rolPropuesto, err := dominio.RolDesde(cmd.Rol)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	idEjecutor, err := dominio.IDUsuarioDesde(cmd.IDSujeto)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}

	decision, err := autorizar(ctx, c.autorizador, cmd.IDSujeto, cmd.IDOrganizacion, dominio.PermisoMiembroInvitar, cmd.Origen)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	rolEjecutor, err := dominio.RolDesde(decision.Rol)
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	if err := dominio.ValidarOtorgamiento(rolEjecutor, rolPropuesto); err != nil {
		return puertos.VistaMiembro{}, err
	}

	sujeto, err := c.sujetos.EsElegible(ctx, idObjetivo.String())
	if err != nil {
		return puertos.VistaMiembro{}, err
	}
	if !sujeto.Existe || !sujeto.Activo {
		return puertos.VistaMiembro{}, &dominio.ErrSujetoNoElegible{}
	}

	var nueva *dominio.Membresia
	var eventos []dominio.EventoDominio
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		org, err := c.organizaciones.CargarParaActualizar(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		if org == nil {
			return &dominio.ErrOrganizacionNoEncontrada{ID: cmd.IDOrganizacion}
		}
		if err := org.EstaOperativa(); err != nil {
			return err
		}

		activas, err := c.membresias.ContarActivasDeOrganizacion(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		if activas >= c.politica.MaximoMiembrosActivos() {
			return &dominio.ErrLimiteMiembrosExcedido{Limite: c.politica.MaximoMiembrosActivos()}
		}

		idMembresia, err := c.ids.NuevoIDMembresia()
		if err != nil {
			return err
		}
		nueva, err = dominio.AgregarMiembro(idMembresia, idOrganizacion, idObjetivo, rolPropuesto, idEjecutor, c.reloj.Ahora())
		if err != nil {
			return err
		}

		if err := c.membresias.Guardar(ctx, nueva); err != nil {
			return err
		}
		eventos = nueva.EventosPendientes()
		for _, e := range eventos {
			if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.VistaMiembro{}, err
	}

	if errPub := c.eventos.Publicar(ctx, eventos...); errPub != nil {
		slog.WarnContext(ctx, "agregar miembro: fallo al publicar eventos",
			"error", errPub, "organizacion_id", idOrganizacion.String())
	}
	return vistaMiembroDesde(nueva), nil
}
