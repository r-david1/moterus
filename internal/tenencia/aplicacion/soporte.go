package aplicacion

// Fixtures y helpers compartidos entre los casos de uso de este paquete:
// construcción de la decisión de autorización a partir de
// puertos.VerificadorDeAutorizacion, traducción de una denegación al error
// de dominio correspondiente (INV-TEN-17) y ensamblado de los modelos de
// lectura (VistaX) a partir de los agregados. Mismo criterio que
// acceso/aplicacion/soporte.go.

import (
	"context"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// autorizar invoca puertos.VerificadorDeAutorizacion.Autorizar (el caso de
// uso Autorizar, §3.7 del diseño, consumido aquí como colaborador de salida)
// y traduce una denegación al error de dominio correspondiente: ErrNoEsMiembro
// (404, sin membresía) o ErrNoAutorizado (403, con el motivo y el rol actual)
// — §3.2 paso 1 del diseño. Devuelve la decisión también en el camino feliz
// porque varios casos de uso necesitan el rol efectivo del ejecutor
// (Autorizacion.Rol) para pasarlo a los métodos de negocio que aplican la
// regla de dominancia (CambiarRol, Remover, TransferirPropiedad...).
func autorizar(ctx context.Context, autorizador puertos.VerificadorDeAutorizacion, idSujeto, idOrganizacion string, permiso dominio.Permiso, origen dominio.OrigenSolicitud) (puertos.Autorizacion, error) {
	decision, err := autorizador.Autorizar(ctx, puertos.ConsultaAutorizacion{
		IDUsuario:      idSujeto,
		IDOrganizacion: idOrganizacion,
		Permiso:        permiso.Valor(),
		Origen:         origen,
	})
	if err != nil {
		return puertos.Autorizacion{}, err
	}
	if decision.Permitido {
		return decision, nil
	}
	return decision, errorDeAutorizacion(decision)
}

// errorDeAutorizacion traduce una Autorizacion denegada al tipo de error de
// dominio correspondiente. "sin_membresia" se distingue de los otros tres
// motivos porque se mapea a un error (y, aguas arriba, un código HTTP)
// distinto: 404 en vez de 403 (INV-TEN-17).
func errorDeAutorizacion(d puertos.Autorizacion) error {
	if d.Motivo == dominio.MotivoDenegacionSinMembresia.Valor() {
		return &dominio.ErrNoEsMiembro{}
	}
	motivo, err := dominio.MotivoDenegacionDesde(d.Motivo)
	if err != nil {
		// No debería alcanzarse: VerificadorDeAutorizacion solo produce
		// motivos del catálogo cerrado. Se conserva un valor por defecto
		// razonable en vez de entrar en pánico.
		motivo = dominio.MotivoDenegacionRolInsuficiente
	}
	return &dominio.ErrNoAutorizado{Motivo: motivo, RolActual: d.Rol}
}

// claveCuentaUsuario construye la ClaveCuenta "usuario:<id>" que
// CrearOrganizacion usa para evaluar Confianza (§3.1 paso 1 del diseño).
func claveCuentaUsuario(id dominio.IDUsuario) string { return "usuario:" + id.String() }

// claveCuentaOrganizacion construye la ClaveCuenta "organizacion:<id>" que
// InvitarMiembro usa para evaluar Confianza: el riesgo de abuso (relay de
// spam por correo) es una propiedad de la organización que invita, no solo
// de quien ejecuta la invitación en un momento dado.
func claveCuentaOrganizacion(id dominio.IDOrganizacion) string { return "organizacion:" + id.String() }

// claveCuentaPorIP construye la ClaveCuenta que AceptarInvitacion usa para
// evaluar Confianza: en ese punto el endpoint es, como la renovación de
// refresco de Acceso (§3.2 de su diseño), un oráculo de fuerza bruta sobre un
// secreto opaco, así que la clave de rate limit relevante es el origen de la
// solicitud y no el usuario (que además todavía no se sabe si es el
// destinatario legítimo).
func claveCuentaPorIP(origen dominio.OrigenSolicitud) string { return "ip:" + origen.IP().String() }

// vistaOrganizacionDesde ensambla el modelo de lectura VistaOrganizacion a
// partir del agregado y el conteo de miembros activos (una consulta aparte:
// Organizacion no conoce sus membresías, §1.2 del diseño).
func vistaOrganizacionDesde(o *dominio.Organizacion, miembrosActivos int) puertos.VistaOrganizacion {
	return puertos.VistaOrganizacion{
		ID:              o.ID().String(),
		Alias:           o.Alias().Normalizado(),
		Nombre:          o.Nombre().Valor(),
		Estado:          o.Estado().String(),
		CreadaEn:        o.CreadaEn(),
		ActualizadaEn:   o.ActualizadaEn(),
		MiembrosActivos: miembrosActivos,
	}
}

// vistaMiembroDesde ensambla el modelo de lectura VistaMiembro a partir del
// agregado Membresia. Deliberadamente no compone correo ni nombre del
// usuario (INV-TEN-29, §2.1 del diseño).
func vistaMiembroDesde(m *dominio.Membresia) puertos.VistaMiembro {
	return puertos.VistaMiembro{
		IDMembresia: m.ID().String(),
		IDUsuario:   m.UsuarioID().String(),
		Rol:         m.Rol().Valor(),
		Estado:      m.Estado().String(),
		CreadaEn:    m.CreadaEn(),
	}
}

// vistaInvitacionDesde ensambla el modelo de lectura VistaInvitacion a
// partir del agregado Invitacion. Nunca incluye el token en claro
// (INV-TEN-23): el tipo VistaInvitacion ni siquiera tiene un campo para él.
func vistaInvitacionDesde(i *dominio.Invitacion) puertos.VistaInvitacion {
	return puertos.VistaInvitacion{
		ID:             i.ID().String(),
		IDOrganizacion: i.OrganizacionID().String(),
		Destinatario:   i.Destinatario().Normalizado(),
		Rol:            i.RolPropuesto().Valor(),
		Estado:         i.Estado().String(),
		CreadaEn:       i.CreadaEn(),
		ExpiraEn:       i.ExpiraEn(),
	}
}
