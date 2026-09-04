// Package tenencia es el ACL de Identidad sobre el contexto Tenencia:
// único paquete de Identidad autorizado a importar tenencia/puertos
// (mismo criterio que acceso/adaptadores/identidad e
// identidad/adaptadores/confianza — "la capa anticorrupción la posee quien
// depende, no quien es dependido"). Implementa
// identidad/puertos.AutorizadorDeConsultas sobre
// tenencia/puertos.VerificadorDeAutorizacion y
// tenencia/puertos.ConsultorDeMembresias (§11.2 del diseño de Tenencia).
package tenencia

import (
	"context"

	identidadpuertos "github.com/r-david1/moterus/internal/identidad/puertos"
	tenenciadominio "github.com/r-david1/moterus/internal/tenencia/dominio"
	tenenciapuertos "github.com/r-david1/moterus/internal/tenencia/puertos"
)

// permisoMiembroVer es el permiso del catálogo cerrado de Tenencia que este
// ACL exige del solicitante. Se declara como constante local (no se importa
// tenencia/dominio.PermisoMiembroVer.Valor() en tiempo de ejecución para no
// acoplar este ACL a más superficie de tenencia/dominio de la
// estrictamente necesaria) y se verifica contra el VO real en un test para
// que no se desincronice en silencio si el catálogo cambia.
const permisoMiembroVer = "miembro.ver"

// AutorizadorConsultas implementa identidad/puertos.AutorizadorDeConsultas.
type AutorizadorConsultas struct {
	autorizador tenenciapuertos.VerificadorDeAutorizacion
	membresias  tenenciapuertos.ConsultorDeMembresias
}

var _ identidadpuertos.AutorizadorDeConsultas = (*AutorizadorConsultas)(nil)

// NuevoAutorizadorConsultas construye el ACL sobre los dos puertos públicos
// que Tenencia expone a otros contextos (§2.4 de su diseño).
func NuevoAutorizadorConsultas(
	autorizador tenenciapuertos.VerificadorDeAutorizacion,
	membresias tenenciapuertos.ConsultorDeMembresias,
) *AutorizadorConsultas {
	return &AutorizadorConsultas{autorizador: autorizador, membresias: membresias}
}

// PuedeConsultar implementa la regla exacta del §11.2 del diseño de
// Tenencia: el solicitante debe tener miembro.ver en la organización Y el
// objetivo debe ser miembro de esa misma organización. Ninguna de las dos
// llamadas es enumerable por correo (ambas resuelven por ID), y ninguna
// revela al llamador el motivo de una denegación de Tenencia: cualquier
// resultado que no sea "ambas condiciones cumplidas" se colapsa en false,
// que el adaptador HTTP de Identidad traduce siempre al mismo 404.
func (a *AutorizadorConsultas) PuedeConsultar(ctx context.Context, idSolicitante, idObjetivo, idOrganizacion string) (bool, error) {
	autorizacion, err := a.autorizador.Autorizar(ctx, tenenciapuertos.ConsultaAutorizacion{
		IDUsuario:      idSolicitante,
		IDOrganizacion: idOrganizacion,
		Permiso:        permisoMiembroVer,
	})
	if err != nil {
		return false, err
	}
	if !autorizacion.Permitido {
		return false, nil
	}

	rol, err := a.membresias.RolEnOrganizacion(ctx, tenenciapuertos.ConsultaRolEnOrganizacion{
		IDUsuario:      idObjetivo,
		IDOrganizacion: idOrganizacion,
	})
	if err != nil {
		return false, err
	}
	return rol.EsMiembro, nil
}

// permisoMiembroVerCoincideConElCatalogo existe solo para que un test de
// este paquete pueda verificar, sin duplicar el catálogo cerrado en el
// test mismo, que la constante local no se desincronizó de
// tenencia/dominio.PermisoMiembroVer.
func permisoMiembroVerCoincideConElCatalogo() bool {
	return permisoMiembroVer == tenenciadominio.PermisoMiembroVer.Valor()
}
