package identidad

import (
	"context"

	accesodominio "github.com/r-david1/moterus/internal/acceso/dominio"
	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
	identidadpuertos "github.com/r-david1/moterus/internal/identidad/puertos"
)

// VerificadorOTP implementa acceso/puertos.VerificadorSegundoFactor sobre
// identidad/puertos.VerificadorOTP (el caso de uso VerificarOTP de
// Identidad, §3.4/§3.6 del diseño otp-mfa.md). Mismo criterio que
// AutenticadorIdentidad/ConsultorEstadoSujeto de este mismo paquete: Acceso
// llama al caso de uso de Identidad, nunca a su repositorio directamente,
// y nunca ve un tipo de identidad/dominio ni identidad/puertos fuera de
// este ACL (INV-ACC-19).
type VerificadorOTP struct {
	verificador identidadpuertos.VerificadorOTP
}

var _ accesopuertos.VerificadorSegundoFactor = (*VerificadorOTP)(nil)

// NuevoVerificadorOTP construye el ACL sobre un
// identidad/puertos.VerificadorOTP ya ensamblado.
func NuevoVerificadorOTP(verificador identidadpuertos.VerificadorOTP) *VerificadorOTP {
	return &VerificadorOTP{verificador: verificador}
}

// Verificar traduce acceso/dominio.OrigenSolicitud ->
// identidad/dominio.OrigenSolicitud y delega en
// identidad/puertos.VerificadorOTP.Verificar. Deliberadamente angosto: solo
// propaga el booleano (INV-MFA-08) — ningún error de Identidad se traduce a
// un tipo de acceso/dominio aquí, porque CompletarSegundoFactorCasoDeUso ya
// colapsa cualquier fallo (error de infraestructura o código incorrecto) en
// el mismo ErrCredencialesRechazadas genérico.
func (a *VerificadorOTP) Verificar(ctx context.Context, idUsuario string, codigo string, origen accesodominio.OrigenSolicitud) (bool, error) {
	return a.verificador.Verificar(ctx, identidadpuertos.ConsultaVerificarOTP{
		IDUsuario: idUsuario,
		Codigo:    codigo,
		Origen:    origenIdentidadDesde(origen),
	})
}
