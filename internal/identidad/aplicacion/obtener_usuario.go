package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// ObtenerUsuarioCasoDeUso implementa puertos.ConsultorDeUsuarios (sección
// 3.3 del diseño). Devuelve siempre un modelo de lectura (VistaUsuario),
// nunca el agregado dominio.Usuario: así el filtrado de campos sensibles
// (p. ej. el hash de contraseña) es estructural, no disciplinario.
//
// La autorización (¿puede este solicitante ver a este usuario?) no se
// resuelve aquí: depende de roles, que son de Tenencia. Se resuelve en la
// orquestación/middleware de Acceso; este caso de uso solo registra quién
// preguntó.
type ObtenerUsuarioCasoDeUso struct {
	usuarios  puertos.RepositorioUsuarios
	auditoria puertos.RegistroAuditoria
	reloj     puertos.Reloj
}

var _ puertos.ConsultorDeUsuarios = (*ObtenerUsuarioCasoDeUso)(nil)

// NuevoObtenerUsuarioCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoObtenerUsuarioCasoDeUso(
	usuarios puertos.RepositorioUsuarios,
	auditoria puertos.RegistroAuditoria,
	reloj puertos.Reloj,
) *ObtenerUsuarioCasoDeUso {
	return &ObtenerUsuarioCasoDeUso{usuarios: usuarios, auditoria: auditoria, reloj: reloj}
}

// ObtenerPorID busca un usuario por ID y lo proyecta a VistaUsuario.
// Auditoría condicional (sección 3.3): si IDSolicitante != IDUsuario y no
// está vacío, se emite usuario.consultado. Consultar el perfil propio no
// se audita (sería ruido que degrada la señal de la bitácora). Un fallo al
// registrar esa auditoría no bloquea la consulta: a diferencia del
// registro y la autenticación (INV-ID-15), ObtenerUsuario no muta estado
// de negocio, por lo que no hay nada que "abortar" — se registra en logs
// de aplicación y se continúa.
func (c *ObtenerUsuarioCasoDeUso) ObtenerPorID(ctx context.Context, q puertos.ConsultaUsuarioPorID) (puertos.VistaUsuario, error) {
	id, err := dominio.IDUsuarioDesde(q.IDUsuario)
	if err != nil {
		return puertos.VistaUsuario{}, err
	}

	usuario, err := c.usuarios.BuscarPorID(ctx, id)
	if err != nil {
		return puertos.VistaUsuario{}, err
	}
	if usuario == nil {
		return puertos.VistaUsuario{}, &dominio.ErrUsuarioNoEncontrado{IDUsuario: q.IDUsuario}
	}

	if q.IDSolicitante != "" && q.IDSolicitante != usuario.ID().String() {
		evento := dominio.NuevoUsuarioConsultado(usuario.ID().String(), q.IDSolicitante, c.reloj.Ahora())
		if errAud := c.auditoria.Registrar(ctx, evento, q.Origen); errAud != nil {
			slog.WarnContext(ctx, "auditoría de usuario.consultado falló",
				"error", errAud, "usuario_id", usuario.ID().String(), "solicitante_id", q.IDSolicitante)
		}
	}

	return vistaUsuarioDesde(usuario), nil
}

func vistaUsuarioDesde(u *dominio.Usuario) puertos.VistaUsuario {
	vista := puertos.VistaUsuario{
		ID:       u.ID().String(),
		Correo:   u.Correo().Normalizado(),
		Estado:   u.Estado().String(),
		TieneMFA: u.TieneMFA(),
		CreadoEn: u.CreadoEn(),
	}
	if t, ok := u.UltimoAccesoEn(); ok {
		copia := t
		vista.UltimoAccesoEn = &copia
	}
	return vista
}
