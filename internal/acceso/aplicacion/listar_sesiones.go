package aplicacion

import (
	"context"

	"github.com/r-david1/moterus/internal/acceso/dominio"
	"github.com/r-david1/moterus/internal/acceso/puertos"
)

// ListarSesionesCasoDeUso implementa puertos.ConsultorDeSesiones (sección
// 3.5 del diseño): la pantalla de "dispositivos conectados". Devuelve
// siempre un modelo de lectura (VistaSesion), nunca el agregado
// dominio.Sesion, así el filtrado de campos sensibles (hashes de
// refresco, cadena de rotación) es estructural, no disciplinario. No se
// audita: es el equivalente a consultar el perfil propio.
type ListarSesionesCasoDeUso struct {
	sesiones puertos.RepositorioSesiones
}

var _ puertos.ConsultorDeSesiones = (*ListarSesionesCasoDeUso)(nil)

// NuevoListarSesionesCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoListarSesionesCasoDeUso(sesiones puertos.RepositorioSesiones) *ListarSesionesCasoDeUso {
	return &ListarSesionesCasoDeUso{sesiones: sesiones}
}

// ListarDeUsuario devuelve las sesiones activas del usuario, marcando cuál
// es la sesión actual (q.IDSesionActual) para que el cliente pueda
// distinguirla en la interfaz.
func (c *ListarSesionesCasoDeUso) ListarDeUsuario(ctx context.Context, q puertos.ConsultaSesionesDeUsuario) ([]puertos.VistaSesion, error) {
	idUsuario, err := dominio.IDUsuarioDesde(q.IDUsuario)
	if err != nil {
		return nil, err
	}

	activas, err := c.sesiones.ListarActivasDeUsuario(ctx, idUsuario)
	if err != nil {
		return nil, err
	}

	vistas := make([]puertos.VistaSesion, 0, len(activas))
	for _, s := range activas {
		if s == nil {
			continue
		}
		vistas = append(vistas, vistaSesionDesde(s, q.IDSesionActual))
	}
	return vistas, nil
}

func vistaSesionDesde(s *dominio.Sesion, idSesionActual string) puertos.VistaSesion {
	vista := puertos.VistaSesion{
		ID:               s.ID().String(),
		EsSesionActual:   s.ID().String() == idSesionActual,
		Estado:           s.Estado().String(),
		CreadaEn:         s.CreadaEn(),
		ExpiraAbsolutoEn: s.ExpiraAbsolutoEn(),
		IPOrigen:         s.OrigenCreacion().IP().String(),
		AgenteUsuario:    s.OrigenCreacion().AgenteUsuario(),
	}
	if t, ok := s.UltimaRenovacionEn(); ok {
		copia := t
		vista.UltimaRenovacionEn = &copia
	}
	return vista
}
