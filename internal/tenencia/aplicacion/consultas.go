package aplicacion

import (
	"context"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// ConsultasCasoDeUso implementa los tres casos de uso de solo lectura de
// §3.6 del diseño: ObtenerOrganizacion y ListarMiembros (puertos.ConsultorDeOrganizaciones
// / parte de puertos.ConsultorDeMembresias, ambos exigen autorización) y
// ListarMisOrganizaciones (no exige autorización de Tenencia, análoga a
// ListarSesiones en Acceso, y no se audita: sería ruido). También implementa
// RolEnOrganizacion, el tercer método de ConsultorDeMembresias: expone los
// hechos crudos de una membresía (rol, estados) sin aplicar la matriz de
// permisos, para que un consumidor que solo necesite "el rol" (p. ej.
// pintarlo en una UI, o poblar Confianza) no tenga que fabricar una
// consulta de autorización falsa. Ninguna de las cuatro operaciones abre
// una UnidadDeTrabajo: son de solo lectura.
type ConsultasCasoDeUso struct {
	organizaciones puertos.RepositorioOrganizaciones
	membresias     puertos.RepositorioMembresias
	autorizador    puertos.VerificadorDeAutorizacion
}

var _ puertos.ConsultorDeOrganizaciones = (*ConsultasCasoDeUso)(nil)
var _ puertos.ConsultorDeMembresias = (*ConsultasCasoDeUso)(nil)

// NuevoConsultasCasoDeUso construye el caso de uso con sus dependencias
// inyectadas por puerto.
func NuevoConsultasCasoDeUso(
	organizaciones puertos.RepositorioOrganizaciones,
	membresias puertos.RepositorioMembresias,
	autorizador puertos.VerificadorDeAutorizacion,
) *ConsultasCasoDeUso {
	return &ConsultasCasoDeUso{
		organizaciones: organizaciones,
		membresias:     membresias,
		autorizador:    autorizador,
	}
}

// ObtenerPorID implementa ObtenerOrganizacion (§3.6 del diseño): exige
// organizacion.ver y devuelve el modelo de lectura VistaOrganizacion, nunca
// el agregado.
func (c *ConsultasCasoDeUso) ObtenerPorID(ctx context.Context, q puertos.ConsultaOrganizacionPorID) (puertos.VistaOrganizacion, error) {
	if _, err := autorizar(ctx, c.autorizador, q.IDSujeto, q.IDOrganizacion, dominio.PermisoOrganizacionVer, q.Origen); err != nil {
		return puertos.VistaOrganizacion{}, err
	}

	idOrganizacion, err := dominio.IDOrganizacionDesde(q.IDOrganizacion)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	org, err := c.organizaciones.BuscarPorID(ctx, idOrganizacion)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	if org == nil {
		return puertos.VistaOrganizacion{}, &dominio.ErrOrganizacionNoEncontrada{ID: q.IDOrganizacion}
	}
	miembrosActivos, err := c.membresias.ContarActivasDeOrganizacion(ctx, idOrganizacion)
	if err != nil {
		return puertos.VistaOrganizacion{}, err
	}
	return vistaOrganizacionDesde(org, miembrosActivos), nil
}

// ListarMiembros implementa el caso de uso homónimo (§3.6 del diseño):
// exige miembro.ver y devuelve modelos de lectura VistaMiembro, sin correo
// ni nombre de usuario (INV-TEN-29, §2.1 del diseño).
func (c *ConsultasCasoDeUso) ListarMiembros(ctx context.Context, q puertos.ConsultaMiembrosDeOrganizacion) ([]puertos.VistaMiembro, error) {
	if _, err := autorizar(ctx, c.autorizador, q.IDSujeto, q.IDOrganizacion, dominio.PermisoMiembroVer, q.Origen); err != nil {
		return nil, err
	}

	idOrganizacion, err := dominio.IDOrganizacionDesde(q.IDOrganizacion)
	if err != nil {
		return nil, err
	}
	membresias, err := c.membresias.ListarDeOrganizacion(ctx, idOrganizacion)
	if err != nil {
		return nil, err
	}
	vistas := make([]puertos.VistaMiembro, 0, len(membresias))
	for _, m := range membresias {
		vistas = append(vistas, vistaMiembroDesde(m))
	}
	return vistas, nil
}

// ListarOrganizacionesDeUsuario implementa ListarMisOrganizaciones (§3.6
// del diseño): la consulta del propio sujeto, sin autorización de Tenencia
// y sin auditoría (sería ruido que degrada la señal de la bitácora).
// IncluirNoActivas=false filtra a las filas donde tanto la membresía como
// la organización están activas; IncluirNoActivas=true devuelve todas.
func (c *ConsultasCasoDeUso) ListarOrganizacionesDeUsuario(ctx context.Context, q puertos.ConsultaOrganizacionesDeUsuario) ([]puertos.VistaMembresiaDeUsuario, error) {
	idUsuario, err := dominio.IDUsuarioDesde(q.IDUsuario)
	if err != nil {
		return nil, err
	}
	membresias, err := c.membresias.ListarDeUsuario(ctx, idUsuario)
	if err != nil {
		return nil, err
	}

	vistas := make([]puertos.VistaMembresiaDeUsuario, 0, len(membresias))
	for _, m := range membresias {
		org, err := c.organizaciones.BuscarPorID(ctx, m.OrganizacionID())
		if err != nil {
			return nil, err
		}
		if org == nil {
			continue
		}
		membresiaActiva := m.Estado().EsIgual(dominio.EstadoMembresiaActiva)
		orgActiva := org.Estado().EsIgual(dominio.EstadoOrganizacionActiva)
		if !q.IncluirNoActivas && !(membresiaActiva && orgActiva) {
			continue
		}
		vistas = append(vistas, puertos.VistaMembresiaDeUsuario{
			IDOrganizacion:     org.ID().String(),
			Alias:              org.Alias().Normalizado(),
			Nombre:             org.Nombre().Valor(),
			Rol:                m.Rol().Valor(),
			EstadoMembresia:    m.Estado().String(),
			EstadoOrganizacion: org.Estado().String(),
		})
	}
	return vistas, nil
}

// RolEnOrganizacion implementa el tercer método de ConsultorDeMembresias
// (§2.1 del diseño). No aplica la matriz de permisos ni requiere
// autorización: es un consumidor confiable dentro del propio sistema (p.
// ej. otro contexto vía su ACL) quien decide qué hacer con el hecho crudo.
func (c *ConsultasCasoDeUso) RolEnOrganizacion(ctx context.Context, q puertos.ConsultaRolEnOrganizacion) (puertos.VistaRolEfectivo, error) {
	idUsuario, err := dominio.IDUsuarioDesde(q.IDUsuario)
	if err != nil {
		return puertos.VistaRolEfectivo{}, err
	}
	idOrganizacion, err := dominio.IDOrganizacionDesde(q.IDOrganizacion)
	if err != nil {
		return puertos.VistaRolEfectivo{}, err
	}

	membresia, err := c.membresias.BuscarVigente(ctx, idUsuario, idOrganizacion)
	if err != nil {
		return puertos.VistaRolEfectivo{}, err
	}
	org, err := c.organizaciones.BuscarPorID(ctx, idOrganizacion)
	if err != nil {
		return puertos.VistaRolEfectivo{}, err
	}
	estadoOrg := ""
	if org != nil {
		estadoOrg = org.Estado().String()
	}

	if membresia == nil {
		return puertos.VistaRolEfectivo{EsMiembro: false, EstadoOrganizacion: estadoOrg}, nil
	}
	return puertos.VistaRolEfectivo{
		EsMiembro:          true,
		Rol:                membresia.Rol().Valor(),
		EstadoMembresia:    membresia.Estado().String(),
		EstadoOrganizacion: estadoOrg,
	}, nil
}
