package aplicacion

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// EvaluarTrustSignalCasoDeUso implementa puertos.EvaluadorDeRiesgo: dos
// niveles de rate limiting simultáneos (IP y cuenta, ADR 0018) más
// verificación de captcha invisible. El orden es normativo:
//
//  1. Si viene TokenCaptcha, se verifica primero (se necesita el puntaje
//     tanto para decidir un posible bypass del límite por cuenta como
//     para el gate final de puntaje).
//  2. Límite por IP: bloqueo duro, sin bypass por captcha — protege
//     contra floods puramente volumétricos desde un origen, algo que
//     "resolver un captcha" no mitiga (un script puede resolver un
//     captcha una vez y seguir).
//  3. Límite por cuenta: bloqueo blando — si no hay captcha válido con
//     puntaje aceptable, se deniega pidiendo uno (RequiereCaptcha=true);
//     si sí lo hay, se deja pasar pese al límite (una persona real
//     reintentando su propia cuenta no debe quedar atascada 15 minutos).
//  4. Gate final de puntaje: si se verificó un captcha y el puntaje no es
//     aceptable, se deniega aunque ningún límite se haya superado
//     (defensa en profundidad).
type EvaluarTrustSignalCasoDeUso struct {
	limitador puertos.LimitadorTasa
	captcha   puertos.VerificadorCaptcha
	politica  dominio.PoliticaLimites
}

var _ puertos.EvaluadorDeRiesgo = (*EvaluarTrustSignalCasoDeUso)(nil)

// NuevoEvaluarTrustSignalCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto y la política de umbrales por
// defecto (dominio.PoliticaLimitesPorDefecto).
func NuevoEvaluarTrustSignalCasoDeUso(limitador puertos.LimitadorTasa, captcha puertos.VerificadorCaptcha) *EvaluarTrustSignalCasoDeUso {
	return &EvaluarTrustSignalCasoDeUso{
		limitador: limitador,
		captcha:   captcha,
		politica:  dominio.PoliticaLimitesPorDefecto(),
	}
}

// Evaluar ejecuta el flujo descrito en el comentario del tipo.
func (c *EvaluarTrustSignalCasoDeUso) Evaluar(ctx context.Context, s Solicitud) (dominio.Decision, error) {
	limites := c.politica.Para(s.Accion)

	huboToken := s.TokenCaptcha != ""
	puntaje := 1.0 // sin captcha evaluado, no se penaliza el puntaje reportado.
	if huboToken {
		p, err := c.captcha.Verificar(ctx, s.TokenCaptcha, string(s.Accion), s.IPOrigen)
		if err != nil {
			// Fail-closed deliberado (a diferencia de HIBP en Identidad):
			// un captcha que no se pudo verificar se trata como "no
			// verificado" (puntaje 0), no como "ausente" — un atacante no
			// debe poder anular la verificación provocando errores de red
			// contra el proveedor.
			slog.WarnContext(ctx, "confianza: verificación de captcha falló; se trata como puntaje 0 (fail-closed)",
				"error", err, "accion", s.Accion)
			puntaje = 0
		} else {
			puntaje = p
		}
	}

	// 2. Límite por IP: bloqueo duro.
	if s.IPOrigen != "" {
		permitido, _, reintentarEn, err := c.limitador.Permitir(ctx, claveLimite("ip", s.Accion, s.IPOrigen), limites.IP)
		if err != nil {
			// Fail-open: una caída de Redis no puede tumbar login/registro
			// por completo (mismo criterio que VerificadorContrasenasFiltradas
			// en Identidad). Se deja constancia en logs para que sea
			// observable, no silenciosa.
			slog.WarnContext(ctx, "confianza: limitador de tasa por IP no disponible; continuando fail-open",
				"error", err, "accion", s.Accion)
		} else if !permitido {
			return dominio.Decision{
				Permitido:    false,
				Motivo:       "limite_ip_excedido",
				ReintentarEn: reintentarEn,
				Puntaje:      puntaje,
			}, nil
		}
	}

	// 3. Límite por cuenta: bloqueo blando, con bypass por captcha válido.
	if s.CorreoNormalizado != "" {
		permitido, _, reintentarEn, err := c.limitador.Permitir(ctx, claveLimite("cuenta", s.Accion, s.CorreoNormalizado), limites.Cuenta)
		if err != nil {
			slog.WarnContext(ctx, "confianza: limitador de tasa por cuenta no disponible; continuando fail-open",
				"error", err, "accion", s.Accion)
		} else if !permitido {
			aceptable, _ := dominio.EvaluarPuntajeCaptcha(puntaje)
			if !huboToken || !aceptable {
				motivo := "limite_cuenta_excedido_requiere_captcha"
				if huboToken {
					motivo = "limite_cuenta_excedido_captcha_insuficiente"
				}
				return dominio.Decision{
					Permitido:       false,
					RequiereCaptcha: true,
					Motivo:          motivo,
					ReintentarEn:    reintentarEn,
					Puntaje:         puntaje,
				}, nil
			}
			// Bypass: humano verificado con buen puntaje pese al cooldown
			// de la cuenta. Cae al gate final de puntaje (paso 4), que ya
			// va a aprobar porque aceptable==true.
		}
	}

	// 4. Gate final de puntaje (defensa en profundidad, independiente de
	// si algún límite se superó).
	if huboToken {
		aceptable, sospechoso := dominio.EvaluarPuntajeCaptcha(puntaje)
		if !aceptable {
			if sospechoso {
				return dominio.Decision{Permitido: false, Motivo: "captcha_puntaje_sospechoso", Puntaje: puntaje}, nil
			}
			return dominio.Decision{Permitido: false, Motivo: "captcha_puntaje_bajo", Puntaje: puntaje}, nil
		}
	}

	return dominio.Decision{Permitido: true, Puntaje: puntaje}, nil
}

// RegistrarResultado resetea el contador de cuenta tras un intento
// exitoso (no penaliza a quien se equivocó una vez y luego acertó). Un
// intento fallido no hace nada adicional aquí: el propio Evaluar ya
// consumió presupuesto de la ventana en la llamada previa (INV-ID-12: se
// evalúa antes de cada intento, éxito o fracaso), así que no hace falta
// una segunda escritura de contador para "fallos" — evita el problema
// clásico de bucket duplicado que se corrige en direcciones opuestas.
func (c *EvaluarTrustSignalCasoDeUso) RegistrarResultado(ctx context.Context, r ResultadoIntento) error {
	if !r.Exitoso || r.CorreoNormalizado == "" {
		return nil
	}
	if err := c.limitador.Reiniciar(ctx, claveLimite("cuenta", r.Accion, r.CorreoNormalizado)); err != nil {
		return fmt.Errorf("confianza: no se pudo reiniciar el contador de cuenta tras un intento exitoso: %w", err)
	}
	return nil
}

// claveLimite arma la clave de Redis (o de cualquier LimitadorTasa) para
// un nivel (ip|cuenta) + acción + identificador. Con un separador estable
// para que test/carga pueda predecir las claves sin acoplarse a un string
// interno distinto en cada sitio.
func claveLimite(nivel string, accion dominio.Accion, identificador string) string {
	return fmt.Sprintf("confianza:rl:%s:%s:%s", nivel, accion, identificador)
}
