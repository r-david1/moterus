// Package identidad es el ACL de Tenencia sobre el contexto Identidad:
// único paquete de Tenencia autorizado a importar identidad/puertos
// (INV-TEN-28, mismo patrón que acceso/adaptadores/identidad e
// identidad/adaptadores/confianza — "la capa anticorrupción la posee quien
// depende, no quien es dependido"). Traduce
// identidad/puertos.ConsultorDeUsuarios (el caso de uso ObtenerUsuario, ya
// construido) a tenencia/puertos.VerificadorDeSujetos, deliberadamente
// ESTRECHO: Tenencia solo necesita saber si el sujeto existe, si está
// activo y, en el flujo de aceptación de invitaciones, su correo
// verificado — nunca el resto de VistaUsuario (§2.2 del diseño).
package identidad

import (
	"context"
	"errors"
	"fmt"

	identidaddominio "github.com/r-david1/moterus/internal/identidad/dominio"
	identidadpuertos "github.com/r-david1/moterus/internal/identidad/puertos"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// estadoActivo es el valor canónico de identidad/dominio.EstadoActivo,
// duplicado aquí a propósito (INV-TEN-27/28: este ACL no importa
// identidad/dominio más allá de lo estrictamente necesario para traducir
// errores tipados, y desde luego tenencia/dominio jamás lo hace).
const estadoActivo = "activo"

// VerificadorSujetos implementa tenencia/puertos.VerificadorDeSujetos sobre
// identidad/puertos.ConsultorDeUsuarios.
type VerificadorSujetos struct {
	consultor identidadpuertos.ConsultorDeUsuarios
}

var _ puertos.VerificadorDeSujetos = (*VerificadorSujetos)(nil)

// NuevoVerificadorSujetos construye el ACL sobre un
// identidad/puertos.ConsultorDeUsuarios ya ensamblado.
func NuevoVerificadorSujetos(consultor identidadpuertos.ConsultorDeUsuarios) *VerificadorSujetos {
	return &VerificadorSujetos{consultor: consultor}
}

// EsElegible consulta a Identidad con IDSolicitante SIEMPRE vacío (llamada
// interna del sistema): así ObtenerUsuarioCasoDeUso.ObtenerPorID no audita
// usuario.consultado en cada validación de elegibilidad (INV-TEN-07) ni en
// cada resolución de correo verificado durante AceptarInvitacion
// (INV-TEN-21) — el mismo criterio, y la misma dependencia dura, que ya usa
// acceso/adaptadores/identidad.ConsultorEstadoSujeto en cada renovación de
// sesión (§2.4 del diseño de Tenencia).
func (v *VerificadorSujetos) EsElegible(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error) {
	vista, err := v.consultor.ObtenerPorID(ctx, identidadpuertos.ConsultaUsuarioPorID{
		IDUsuario:     idUsuario,
		IDSolicitante: "",
	})
	if err != nil {
		var errNoEncontrado *identidaddominio.ErrUsuarioNoEncontrado
		if errors.As(err, &errNoEncontrado) {
			return puertos.SujetoElegible{Existe: false}, nil
		}
		// Cualquier otro error (infraestructura, VO inválido) se envuelve
		// sin `%w`, a propósito: un tipo de identidad/dominio nunca debe
		// cruzar hacia tenencia/aplicacion (INV-TEN-28).
		return puertos.SujetoElegible{}, fmt.Errorf("tenencia/adaptadores/identidad: fallo al consultar la elegibilidad del sujeto: %v", err)
	}
	return puertos.SujetoElegible{
		Existe:            true,
		Activo:            vista.Estado == estadoActivo,
		CorreoNormalizado: vista.Correo,
	}, nil
}
