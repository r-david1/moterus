package identidad

import (
	"context"
	"errors"
	"fmt"

	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
	identidaddominio "github.com/r-david1/moterus/internal/identidad/dominio"
	identidadpuertos "github.com/r-david1/moterus/internal/identidad/puertos"
)

// ConsultorEstadoSujeto implementa acceso/puertos.ConsultorEstadoSujeto
// sobre identidad/puertos.ConsultorDeUsuarios (el caso de uso
// ObtenerUsuario de Identidad). Se invoca en CADA renovación (§3.2 paso 5
// del diseño): es el mecanismo por el que Acceso se entera de que
// Identidad suspendió o bloqueó una cuenta sin necesidad de un broker de
// eventos (ADR candidato 0022).
//
// identidad/puertos.VistaUsuario no expone hoy un campo con la semántica
// correcta de "cuándo cambió la contraseña" (§11.1 del diseño: hallazgo ya
// documentado, sin impacto en el MVP porque CambiarContrasena todavía no
// existe en Identidad), así que EstadoSujeto.CredencialActualizadaEn queda
// siempre en cero — ningún caso de uso de este hito lo lee.
type ConsultorEstadoSujeto struct {
	consultor identidadpuertos.ConsultorDeUsuarios
}

var _ accesopuertos.ConsultorEstadoSujeto = (*ConsultorEstadoSujeto)(nil)

// NuevoConsultorEstadoSujeto construye el ACL sobre un
// identidad/puertos.ConsultorDeUsuarios ya ensamblado.
func NuevoConsultorEstadoSujeto(consultor identidadpuertos.ConsultorDeUsuarios) *ConsultorEstadoSujeto {
	return &ConsultorEstadoSujeto{consultor: consultor}
}

// EstadoDe consulta a Identidad con IDSolicitante vacío (llamada interna
// del sistema): así ObtenerUsuarioCasoDeUso.ObtenerPorID no audita
// usuario.consultado en cada renovación (comportamiento actual del caso de
// uso de Identidad — se depende de él explícitamente, ver §11.1 del
// diseño), lo que inundaría la cadena de auditoría serializada por el
// advisory lock (ADR 0005).
func (c *ConsultorEstadoSujeto) EstadoDe(ctx context.Context, idUsuario string) (accesopuertos.EstadoSujeto, error) {
	vista, err := c.consultor.ObtenerPorID(ctx, identidadpuertos.ConsultaUsuarioPorID{
		IDUsuario:     idUsuario,
		IDSolicitante: "",
	})
	if err != nil {
		var errNoEncontrado *identidaddominio.ErrUsuarioNoEncontrado
		if errors.As(err, &errNoEncontrado) {
			return accesopuertos.EstadoSujeto{Existe: false}, nil
		}
		// Cualquier otro error (infraestructura, VO inválido) se envuelve
		// sin `%w`, a propósito: un tipo de identidad/dominio nunca debe
		// cruzar hacia acceso/aplicacion (INV-ACC-19), y este ACL no tiene
		// más traducciones tipadas que ofrecer para ObtenerUsuario.
		return accesopuertos.EstadoSujeto{}, fmt.Errorf("acceso/adaptadores/identidad: fallo al consultar el estado del sujeto: %v", err)
	}
	return accesopuertos.EstadoSujeto{
		Existe: true,
		Estado: vista.Estado,
	}, nil
}
