package aplicacion

import (
	"context"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// ComandoOlvidarPerfilDeOrigen transporta la entrada de
// OlvidarPerfilDeOrigenCasoDeUso.Olvidar (§3.5 del diseño
// fingerprinting-comportamiento.md).
type ComandoOlvidarPerfilDeOrigen struct {
	CorreoNormalizado string
}

// OlvidarPerfilDeOrigenCasoDeUso es el único caso de uso genuinamente
// nuevo de la extensión de reconocimiento de origen (§3.5 del diseño):
// borra el perfil completo de una cuenta en el índice derivado de Redis.
//
// No se audita: borrar un índice derivado no es un hecho de negocio (el
// hecho que sí importa, si alguna vez existe "usuario.anonimizado", lo
// audita Identidad, no Confianza).
//
// Sin endpoint HTTP (§7 del diseño): este producto no tiene rol de
// administrador de plataforma. Sus llamadores previstos son un
// subcomando de operaciones y, más adelante, el gancho de
// usuario.anonimizado de Identidad — ninguno de los dos entra en el
// alcance de esta extensión.
type OlvidarPerfilDeOrigenCasoDeUso struct {
	perfiles puertos.PerfilDeOrigenes
}

// NuevoOlvidarPerfilDeOrigenCasoDeUso construye el caso de uso sobre el
// puerto de reconocimiento de origen ya ensamblado.
func NuevoOlvidarPerfilDeOrigenCasoDeUso(perfiles puertos.PerfilDeOrigenes) *OlvidarPerfilDeOrigenCasoDeUso {
	return &OlvidarPerfilDeOrigenCasoDeUso{perfiles: perfiles}
}

// Olvidar borra el perfil de orígenes de la cuenta identificada por
// cmd.CorreoNormalizado. Idempotente: olvidar un perfil que ya no existe
// (o que nunca existió) no es un error.
func (c *OlvidarPerfilDeOrigenCasoDeUso) Olvidar(ctx context.Context, cmd ComandoOlvidarPerfilDeOrigen) error {
	clave := dominio.NuevaClaveCuenta(cmd.CorreoNormalizado)
	return c.perfiles.Olvidar(ctx, clave.String())
}
