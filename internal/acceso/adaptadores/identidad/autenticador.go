// Package identidad es el ACL (anticorruption layer) de Acceso hacia
// Identidad: el ÚNICO paquete de Acceso autorizado a importar
// identidad/puertos (INV-ACC-19 — verificable con un test de imports en
// test/arquitectura). Traduce en ambos sentidos: acceso/puertos.
// CredencialesSujeto/SolicitudEvaluacion -> identidad/puertos.
// ComandoAutenticar/ConsultaUsuarioPorID, y los errores tipados de
// identidad/dominio -> los de acceso/dominio (§3.1 del diseño, paso 2).
// acceso/aplicacion nunca ve un tipo de Identidad.
package identidad

import (
	"context"
	"errors"

	accesodominio "github.com/r-david1/moterus/internal/acceso/dominio"
	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
	identidaddominio "github.com/r-david1/moterus/internal/identidad/dominio"
	identidadpuertos "github.com/r-david1/moterus/internal/identidad/puertos"
)

// AutenticadorIdentidad implementa acceso/puertos.AutenticadorIdentidad
// sobre identidad/puertos.AutenticadorDeCredenciales (el caso de uso
// AutenticarUsuario de Identidad). Acceso llama al caso de uso de
// Identidad, nunca a su repositorio directamente (§0 del diseño).
type AutenticadorIdentidad struct {
	autenticador identidadpuertos.AutenticadorDeCredenciales
}

var _ accesopuertos.AutenticadorIdentidad = (*AutenticadorIdentidad)(nil)

// NuevoAutenticadorIdentidad construye el ACL sobre un
// identidad/puertos.AutenticadorDeCredenciales ya ensamblado.
func NuevoAutenticadorIdentidad(autenticador identidadpuertos.AutenticadorDeCredenciales) *AutenticadorIdentidad {
	return &AutenticadorIdentidad{autenticador: autenticador}
}

// Autenticar traduce acceso/puertos.CredencialesSujeto ->
// identidad/puertos.ComandoAutenticar, delega, y traduce el resultado y
// los errores tipados de Identidad a los de acceso/dominio (§3.1 del
// diseño, paso 2). Ninguno de estos casos audita en Acceso: Identidad ya
// audita usuario.login con el resultado correspondiente.
func (a *AutenticadorIdentidad) Autenticar(ctx context.Context, c accesopuertos.CredencialesSujeto) (accesopuertos.SujetoAutenticado, error) {
	resultado, err := a.autenticador.Autenticar(ctx, identidadpuertos.ComandoAutenticar{
		Correo:       c.Correo,
		Contrasena:   c.Contrasena,
		Origen:       origenIdentidadDesde(c.Origen),
		TokenCaptcha: c.TokenCaptcha,
	})
	if err != nil {
		return accesopuertos.SujetoAutenticado{}, traducirErrorAutenticacion(err)
	}
	return accesopuertos.SujetoAutenticado{
		IDUsuario:             resultado.IDUsuario,
		Estado:                resultado.Estado,
		RequiereSegundoFactor: resultado.RequiereSegundoFactor,
		MotivoStepUp:          resultado.MotivoStepUp,
	}, nil
}

// traducirErrorAutenticacion es la traducción tipada de §3.1 paso 2 del
// diseño: acceso/aplicacion nunca ve un tipo de identidad/dominio
// (INV-ACC-19). Un error de Identidad que no está en esta lista (no
// debería ocurrir con AutenticarUsuario, que solo produce los tipos de
// abajo) se envuelve sin tipar, para que nunca escape un *identidaddominio.Err*
// hacia acceso/aplicacion.
func traducirErrorAutenticacion(err error) error {
	var (
		errCredenciales     *identidaddominio.ErrCredencialesInvalidas
		errCorreoNoVerif    *identidaddominio.ErrCorreoNoVerificado
		errCuentaSuspendida *identidaddominio.ErrCuentaSuspendida
		errCuentaBloqueada  *identidaddominio.ErrCuentaBloqueada
		errAccesoDenegado   *identidaddominio.ErrAccesoDenegadoPorConfianza
	)

	switch {
	case errors.As(err, &errCredenciales):
		return &accesodominio.ErrCredencialesRechazadas{}
	case errors.As(err, &errCorreoNoVerif):
		return &accesodominio.ErrCuentaNoOperativa{Motivo: accesodominio.MotivoCuentaNoOperativaCorreoNoVerificado}
	case errors.As(err, &errCuentaSuspendida):
		return &accesodominio.ErrCuentaNoOperativa{Motivo: accesodominio.MotivoCuentaNoOperativaSuspendida}
	case errors.As(err, &errCuentaBloqueada):
		return &accesodominio.ErrCuentaNoOperativa{Motivo: accesodominio.MotivoCuentaNoOperativaBloqueada}
	case errors.As(err, &errAccesoDenegado):
		return &accesodominio.ErrAccesoDenegadoPorConfianza{Motivo: errAccesoDenegado.Motivo, ReintentarEn: errAccesoDenegado.ReintentarEn}
	default:
		// No debería ocurrir con AutenticarUsuario (solo produce los tipos
		// de arriba), pero si algún día devuelve algo nuevo, nunca debe
		// escapar un tipo de identidad/dominio hacia acceso/aplicacion
		// (INV-ACC-19): se degrada al genérico de credenciales rechazadas.
		return &accesodominio.ErrCredencialesRechazadas{}
	}
}

// origenIdentidadDesde traduce acceso/dominio.OrigenSolicitud ->
// identidad/dominio.OrigenSolicitud (§1.7 del diseño de Acceso: son tipos
// deliberadamente duplicados, no compartidos). Si la construcción falla
// (no debería: ambos VOs aplican las mismas reglas sobre los mismos datos
// ya validados por el middleware HTTP), se degrada a un OrigenSolicitud
// vacío en vez de bloquear la llamada a Identidad por un problema de
// traducción de un campo forense.
func origenIdentidadDesde(o accesodominio.OrigenSolicitud) identidaddominio.OrigenSolicitud {
	origen, err := identidaddominio.NuevoOrigenSolicitud(o.IP().String(), o.AgenteUsuario(), o.HuellaDispositivo(), o.IDSolicitud())
	if err != nil {
		origen, _ = identidaddominio.NuevoOrigenSolicitud("", o.AgenteUsuario(), o.HuellaDispositivo(), o.IDSolicitud())
	}
	return origen
}
