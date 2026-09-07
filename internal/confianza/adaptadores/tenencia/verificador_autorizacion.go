// Package tenencia es el ACL de Confianza sobre el contexto Tenencia: único
// paquete de Confianza autorizado a importar tenencia/puertos (mismo
// criterio de frontera que INV-TEN-28 e identidad/adaptadores/tenencia —
// "la capa anticorrupción la posee quien depende, no quien es dependido",
// §2.2 del diseño docs/design/colas-virtuales.md). Implementa
// confianza/puertos.VerificadorDeAutorizacion sobre
// tenencia/puertos.VerificadorDeAutorizacion: la misma relación exacta que
// identidad/adaptadores/tenencia.AutorizadorConsultas ya establece para
// Identidad.
package tenencia

import (
	"context"

	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"
	tenenciadominio "github.com/r-david1/moterus/internal/tenencia/dominio"
	tenenciapuertos "github.com/r-david1/moterus/internal/tenencia/puertos"
)

// PermisoOrganizacionEditar / PermisoOrganizacionVer son los dos permisos
// del catálogo cerrado de Tenencia que los endpoints de administración de
// salas de espera de Confianza exigen (§7.2 del diseño
// docs/design/colas-virtuales.md, ADR candidato 0046: "se reutiliza
// organizacion.editar en vez de agregar un noveno permiso"). Se declaran
// como constantes locales de string (no se importa
// tenencia/dominio.Permiso en tiempo de ejecución fuera de este ACL, para
// no acoplar confianza/adaptadores/http a más superficie de tenencia/dominio
// de la estrictamente necesaria — mismo criterio que
// identidad/adaptadores/tenencia.permisoMiembroVer) y se verifican contra
// el VO real en un test de este paquete para que no se desincronicen en
// silencio si el catálogo cambia.
const (
	PermisoOrganizacionEditar = "organizacion.editar"
	PermisoOrganizacionVer    = "organizacion.ver"
)

// permisosCoincidenConElCatalogoDeTenencia existe solo para que un test de
// este paquete pueda verificar, sin duplicar el catálogo cerrado en el test
// mismo, que las constantes de arriba no se desincronizaron de
// tenencia/dominio.PermisoOrganizacionEditar/PermisoOrganizacionVer.
func permisosCoincidenConElCatalogoDeTenencia() bool {
	return PermisoOrganizacionEditar == tenenciadominio.PermisoOrganizacionEditar.Valor() &&
		PermisoOrganizacionVer == tenenciadominio.PermisoOrganizacionVer.Valor()
}

// VerificadorAutorizacion implementa confianza/puertos.VerificadorDeAutorizacion.
type VerificadorAutorizacion struct {
	autorizador tenenciapuertos.VerificadorDeAutorizacion
}

var _ confianzapuertos.VerificadorDeAutorizacion = (*VerificadorAutorizacion)(nil)

// NuevoVerificadorAutorizacion construye el ACL sobre el único puerto
// público de autorización que Tenencia expone a otros contextos.
func NuevoVerificadorAutorizacion(autorizador tenenciapuertos.VerificadorDeAutorizacion) *VerificadorAutorizacion {
	return &VerificadorAutorizacion{autorizador: autorizador}
}

// Autorizar traduce la consulta primitiva de Confianza
// (ConsultaAutorizacionOrganizacion) a tenencia/puertos.ConsultaAutorizacion
// y colapsa la respuesta rica de Tenencia (Autorizacion.Permitido +
// Autorizacion.Motivo) al booleano estrecho que exige
// confianza/puertos.VerificadorDeAutorizacion (comentario de ese puerto:
// "deliberadamente primitivo... Confianza no debe importar el vocabulario
// de tenencia/dominio.Permiso"). Esto significa que este ACL, a propósito,
// NO reexpone la distinción "sin_membresia" (404) vs. "rol_insuficiente"
// (403) que sí usa el propio middleware de autorización de Tenencia
// (middlewareAutorizacionTenencia): el consumidor de este puerto (el
// middleware HTTP de administración de salas de espera de Confianza, §7.2
// del diseño) solo puede distinguir "autorizado" de "no autorizado", nunca
// el motivo. Es una decisión de la propia firma del puerto de Confianza, no
// de este adaptador.
func (v *VerificadorAutorizacion) Autorizar(ctx context.Context, q confianzapuertos.ConsultaAutorizacionOrganizacion) (bool, error) {
	autorizacion, err := v.autorizador.Autorizar(ctx, tenenciapuertos.ConsultaAutorizacion{
		IDUsuario:      q.IDSujeto,
		IDOrganizacion: q.IDOrganizacion,
		Permiso:        q.Permiso,
	})
	if err != nil {
		return false, err
	}
	return autorizacion.Permitido, nil
}
