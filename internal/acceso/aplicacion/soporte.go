package aplicacion

import (
	"time"

	"github.com/r-david1/moterus/internal/acceso/dominio"
)

// hastaRevocacionListaAcceso calcula, a partir del instante de una
// revocación, hasta cuándo debe conservarse el sid en puertos.ListaRevocacion
// (Redis). El acelerador solo necesita cubrir la ventana en la que un token
// de acceso ya emitido podría seguir siendo válido: como mucho
// PoliticaSesion.VidaTokenAcceso() desde ahora (INV-ACC-15). Pasado ese
// punto el propio exp del JWT ya lo habría rechazado.
func hastaRevocacionListaAcceso(ahora time.Time, politica dominio.PoliticaSesion) time.Time {
	return ahora.Add(politica.VidaTokenAcceso())
}

// sesionMasAntigua devuelve, de una lista de sesiones activas, la de menor
// CreadaEn (o nil si la lista está vacía). La usa IniciarSesion para
// aplicar INV-ACC-22: al superar PoliticaSesion.MaximoSesionesActivas(),
// se revoca la sesión activa más antigua, nunca se rechaza el login nuevo.
func sesionMasAntigua(sesiones []*dominio.Sesion) *dominio.Sesion {
	var masAntigua *dominio.Sesion
	for _, s := range sesiones {
		if s == nil {
			continue
		}
		if masAntigua == nil || s.CreadaEn().Before(masAntigua.CreadaEn()) {
			masAntigua = s
		}
	}
	return masAntigua
}

// claveCuentaPorIP construye la ClaveCuenta que RenovarSesion usa para
// evaluar Confianza (§3.2 paso 1 del diseño): en ese punto todavía no se
// resolvió a qué sesión pertenece el token presentado (eso ocurre después,
// en BuscarPorHashRefresco), así que la única clave de rate limit
// disponible es el origen de la solicitud. Prefijo "ip:" para mantener la
// misma forma "<tipo>:<valor>" que describe puertos.SolicitudEvaluacion.ClaveCuenta.
func claveCuentaPorIP(origen dominio.OrigenSolicitud) string {
	return "ip:" + origen.IP().String()
}

// claveCuentaPorUsuario construye la ClaveCuenta que CerrarTodas usa para
// evaluar Confianza (§3.4 del diseño): aquí sí se conoce el usuario.
func claveCuentaPorUsuario(u dominio.IDUsuario) string {
	return "usuario:" + u.String()
}
