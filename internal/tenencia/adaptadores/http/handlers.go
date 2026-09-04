package http

import (
	"context"

	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// ManejadorTenencia agrupa los casos de uso del MVP de Tenencia detrás de
// los puertos de entrada, nunca de los structs concretos de aplicacion.
type ManejadorTenencia struct {
	organizaciones          puertos.GestorDeOrganizaciones
	consultorOrganizaciones puertos.ConsultorDeOrganizaciones
	membresias              puertos.GestorDeMembresias
	consultorMembresias     puertos.ConsultorDeMembresias
	invitaciones            puertos.GestorDeInvitaciones
}

// NuevoManejadorTenencia construye el manejador HTTP con sus puertos de
// entrada inyectados.
func NuevoManejadorTenencia(
	organizaciones puertos.GestorDeOrganizaciones,
	consultorOrganizaciones puertos.ConsultorDeOrganizaciones,
	membresias puertos.GestorDeMembresias,
	consultorMembresias puertos.ConsultorDeMembresias,
	invitaciones puertos.GestorDeInvitaciones,
) *ManejadorTenencia {
	return &ManejadorTenencia{
		organizaciones:          organizaciones,
		consultorOrganizaciones: consultorOrganizaciones,
		membresias:              membresias,
		consultorMembresias:     consultorMembresias,
		invitaciones:            invitaciones,
	}
}

// idSujetoDesdeContexto extrae el `sub` del acceso/puertos.Acceso ya
// validado por middlewareAutenticacionTenencia (INV-TEN-12: el IDSujeto
// ejecutor SIEMPRE sale del token, nunca del cuerpo ni de la query).
func idSujetoDesdeContexto(ctx context.Context) string {
	acceso, _ := AccesoDesdeContexto(ctx)
	return acceso.IDUsuario
}

// --- Organizaciones -----------------------------------------------------------

// CrearOrganizacion implementa el handler Huma de POST /tenencia/organizaciones.
func (m *ManejadorTenencia) CrearOrganizacion(ctx context.Context, in *CrearOrganizacionInput) (*CrearOrganizacionOutput, error) {
	vista, err := m.organizaciones.Crear(ctx, puertos.ComandoCrearOrganizacion{
		Nombre:   in.Body.Nombre,
		Alias:    in.Body.Alias,
		IDSujeto: idSujetoDesdeContexto(ctx),
		Origen:   origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &CrearOrganizacionOutput{Body: vistaOrganizacionRespuestaDesde(vista)}, nil
}

// ListarMisOrganizaciones implementa el handler Huma de GET /tenencia/organizaciones.
func (m *ManejadorTenencia) ListarMisOrganizaciones(ctx context.Context, in *ListarMisOrganizacionesInput) (*ListarMisOrganizacionesOutput, error) {
	vistas, err := m.consultorMembresias.ListarOrganizacionesDeUsuario(ctx, puertos.ConsultaOrganizacionesDeUsuario{
		IDUsuario:        idSujetoDesdeContexto(ctx),
		IncluirNoActivas: in.IncluirNoActivas,
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	body := make([]vistaMembresiaDeUsuarioRespuesta, 0, len(vistas))
	for _, v := range vistas {
		body = append(body, vistaMembresiaDeUsuarioRespuestaDesde(v))
	}
	return &ListarMisOrganizacionesOutput{Body: body}, nil
}

// ObtenerOrganizacion implementa el handler Huma de
// GET /tenencia/organizaciones/{idOrganizacion}.
func (m *ManejadorTenencia) ObtenerOrganizacion(ctx context.Context, in *ObtenerOrganizacionInput) (*ObtenerOrganizacionOutput, error) {
	vista, err := m.consultorOrganizaciones.ObtenerPorID(ctx, puertos.ConsultaOrganizacionPorID{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		Origen:         origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &ObtenerOrganizacionOutput{Body: vistaOrganizacionRespuestaDesde(vista)}, nil
}

// ActualizarOrganizacion implementa el handler Huma de
// PATCH /tenencia/organizaciones/{idOrganizacion}.
func (m *ManejadorTenencia) ActualizarOrganizacion(ctx context.Context, in *ActualizarOrganizacionInput) (*ActualizarOrganizacionOutput, error) {
	vista, err := m.organizaciones.Actualizar(ctx, puertos.ComandoActualizarOrganizacion{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		Nombre:         in.Body.Nombre,
		Alias:          in.Body.Alias,
		Origen:         origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &ActualizarOrganizacionOutput{Body: vistaOrganizacionRespuestaDesde(vista)}, nil
}

// CambiarEstadoOrganizacion implementa el handler Huma de
// POST /tenencia/organizaciones/{idOrganizacion}/cambios-estado.
func (m *ManejadorTenencia) CambiarEstadoOrganizacion(ctx context.Context, in *CambiarEstadoOrganizacionInput) (*CambiarEstadoOrganizacionOutput, error) {
	vista, err := m.organizaciones.CambiarEstado(ctx, puertos.ComandoCambiarEstadoOrganizacion{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		Destino:        in.Body.Destino,
		Motivo:         in.Body.Motivo,
		Origen:         origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &CambiarEstadoOrganizacionOutput{Body: vistaOrganizacionRespuestaDesde(vista)}, nil
}

// --- Membresías ---------------------------------------------------------------

// ListarMiembros implementa el handler Huma de
// GET /tenencia/organizaciones/{idOrganizacion}/miembros.
func (m *ManejadorTenencia) ListarMiembros(ctx context.Context, in *ListarMiembrosInput) (*ListarMiembrosOutput, error) {
	vistas, err := m.consultorMembresias.ListarMiembros(ctx, puertos.ConsultaMiembrosDeOrganizacion{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		Origen:         origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	body := make([]vistaMiembroRespuesta, 0, len(vistas))
	for _, v := range vistas {
		body = append(body, vistaMiembroRespuestaDesde(v))
	}
	return &ListarMiembrosOutput{Body: body}, nil
}

// AgregarMiembro implementa el handler Huma de
// POST /tenencia/organizaciones/{idOrganizacion}/miembros (alta directa,
// §3.3 del diseño).
func (m *ManejadorTenencia) AgregarMiembro(ctx context.Context, in *AgregarMiembroInput) (*AgregarMiembroOutput, error) {
	vista, err := m.membresias.Agregar(ctx, puertos.ComandoAgregarMiembro{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		IDUsuario:      in.Body.IDUsuario,
		Rol:            in.Body.Rol,
		Origen:         origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &AgregarMiembroOutput{Body: vistaMiembroRespuestaDesde(vista)}, nil
}

// CambiarRolMiembro implementa el handler Huma de
// PATCH /tenencia/organizaciones/{idOrganizacion}/miembros/{idUsuario}.
func (m *ManejadorTenencia) CambiarRolMiembro(ctx context.Context, in *CambiarRolInput) (*CambiarRolOutput, error) {
	vista, err := m.membresias.CambiarRol(ctx, puertos.ComandoCambiarRol{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		IDUsuario:      in.IDUsuario,
		NuevoRol:       in.Body.NuevoRol,
		Origen:         origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &CambiarRolOutput{Body: vistaMiembroRespuestaDesde(vista)}, nil
}

// RemoverMiembro implementa el handler Huma de
// DELETE /tenencia/organizaciones/{idOrganizacion}/miembros/{idUsuario}.
func (m *ManejadorTenencia) RemoverMiembro(ctx context.Context, in *RemoverMiembroInput) (*RemoverMiembroOutput, error) {
	if err := m.membresias.Remover(ctx, puertos.ComandoRemoverMiembro{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		IDUsuario:      in.IDUsuario,
		Origen:         origenSolicitudDesdeContexto(ctx),
	}); err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &RemoverMiembroOutput{}, nil
}

// AbandonarOrganizacion implementa el handler Huma de
// DELETE /tenencia/organizaciones/{idOrganizacion}/miembros/actual.
func (m *ManejadorTenencia) AbandonarOrganizacion(ctx context.Context, in *AbandonarOrganizacionInput) (*AbandonarOrganizacionOutput, error) {
	if err := m.membresias.Abandonar(ctx, puertos.ComandoAbandonarOrganizacion{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		Origen:         origenSolicitudDesdeContexto(ctx),
	}); err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &AbandonarOrganizacionOutput{}, nil
}

// TransferirPropiedad implementa el handler Huma de
// POST .../transferencias-propiedad.
func (m *ManejadorTenencia) TransferirPropiedad(ctx context.Context, in *TransferirPropiedadInput) (*TransferirPropiedadOutput, error) {
	if err := m.membresias.TransferirPropiedad(ctx, puertos.ComandoTransferirPropiedad{
		IDOrganizacion:          in.IDOrganizacion,
		IDSujeto:                idSujetoDesdeContexto(ctx),
		IDNuevoPropietario:      in.Body.IDNuevoPropietario,
		RolResultanteDelCedente: in.Body.RolResultanteDelCedente,
		Origen:                  origenSolicitudDesdeContexto(ctx),
	}); err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &TransferirPropiedadOutput{}, nil
}

// --- Invitaciones ---------------------------------------------------------------

// InvitarMiembro implementa el handler Huma de POST .../invitaciones.
// Nunca devuelve el token en claro (INV-TEN-23): vistaInvitacionRespuesta
// no tiene campo para él.
func (m *ManejadorTenencia) InvitarMiembro(ctx context.Context, in *InvitarMiembroInput) (*InvitarMiembroOutput, error) {
	vista, err := m.invitaciones.Invitar(ctx, puertos.ComandoInvitarMiembro{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		Correo:         in.Body.Correo,
		Rol:            in.Body.Rol,
		Origen:         origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &InvitarMiembroOutput{Body: vistaInvitacionRespuestaDesde(vista)}, nil
}

// RevocarInvitacion implementa el handler Huma de
// DELETE .../invitaciones/{idInvitacion}.
func (m *ManejadorTenencia) RevocarInvitacion(ctx context.Context, in *RevocarInvitacionInput) (*RevocarInvitacionOutput, error) {
	if err := m.invitaciones.Revocar(ctx, puertos.ComandoRevocarInvitacion{
		IDOrganizacion: in.IDOrganizacion,
		IDSujeto:       idSujetoDesdeContexto(ctx),
		IDInvitacion:   in.IDInvitacion,
		Origen:         origenSolicitudDesdeContexto(ctx),
	}); err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &RevocarInvitacionOutput{}, nil
}

// AceptarInvitacion implementa el handler Huma de
// POST /tenencia/invitaciones/aceptaciones. No cuelga de
// /organizaciones/{id} (§7 del diseño): quien acepta no es miembro de
// ninguna organización todavía, así que solo pasa por el middleware de
// autenticación, nunca por el de autorización.
func (m *ManejadorTenencia) AceptarInvitacion(ctx context.Context, in *AceptarInvitacionInput) (*AceptarInvitacionOutput, error) {
	vista, err := m.invitaciones.Aceptar(ctx, puertos.ComandoAceptarInvitacion{
		TokenPlano: in.Body.Token,
		IDSujeto:   idSujetoDesdeContexto(ctx),
		Origen:     origenSolicitudDesdeContexto(ctx),
	})
	if err != nil {
		return nil, mapearErrorDominio(ctx, err)
	}
	return &AceptarInvitacionOutput{Body: vistaMiembroRespuestaDesde(vista)}, nil
}

// --- Proyecciones VistaX -> DTO de respuesta ---------------------------------

func vistaOrganizacionRespuestaDesde(v puertos.VistaOrganizacion) vistaOrganizacionRespuesta {
	return vistaOrganizacionRespuesta{
		ID:              v.ID,
		Alias:           v.Alias,
		Nombre:          v.Nombre,
		Estado:          v.Estado,
		CreadaEn:        v.CreadaEn,
		ActualizadaEn:   v.ActualizadaEn,
		MiembrosActivos: v.MiembrosActivos,
	}
}

func vistaMiembroRespuestaDesde(v puertos.VistaMiembro) vistaMiembroRespuesta {
	return vistaMiembroRespuesta{
		IDMembresia: v.IDMembresia,
		IDUsuario:   v.IDUsuario,
		Rol:         v.Rol,
		Estado:      v.Estado,
		CreadaEn:    v.CreadaEn,
	}
}

func vistaMembresiaDeUsuarioRespuestaDesde(v puertos.VistaMembresiaDeUsuario) vistaMembresiaDeUsuarioRespuesta {
	return vistaMembresiaDeUsuarioRespuesta{
		IDOrganizacion:     v.IDOrganizacion,
		Alias:              v.Alias,
		Nombre:             v.Nombre,
		Rol:                v.Rol,
		EstadoMembresia:    v.EstadoMembresia,
		EstadoOrganizacion: v.EstadoOrganizacion,
	}
}

func vistaInvitacionRespuestaDesde(v puertos.VistaInvitacion) vistaInvitacionRespuesta {
	return vistaInvitacionRespuesta{
		ID:             v.ID,
		IDOrganizacion: v.IDOrganizacion,
		Destinatario:   v.Destinatario,
		Rol:            v.Rol,
		Estado:         v.Estado,
		CreadaEn:       v.CreadaEn,
		ExpiraEn:       v.ExpiraEn,
	}
}
