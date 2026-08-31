package aplicacion_test

import (
	"context"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/aplicacion"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

func TestEvaluar_permiteCuandoNadaExcedeLimite(t *testing.T) {
	limitador := &mocks.LimitadorTasa{}
	captcha := &mocks.VerificadorCaptcha{}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, captcha)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !decision.Permitido {
		t.Fatalf("se esperaba Permitido=true, obtuvo %+v", decision)
	}
	if len(limitador.LlamadasPermitir) != 2 {
		t.Fatalf("se esperaban 2 llamadas a Permitir (ip + cuenta), hubo %d", len(limitador.LlamadasPermitir))
	}
}

func TestEvaluar_bloqueaPorLimiteDeIP_sinBypassPorCaptcha(t *testing.T) {
	limitador := &mocks.LimitadorTasa{
		FnPermitir: func(ctx context.Context, clave string, umbral dominio.Umbral) (bool, int, time.Duration, error) {
			if clave == "confianza:rl:ip:login:203.0.113.1" {
				return false, 0, 30 * time.Second, nil
			}
			return true, 5, 0, nil
		},
	}
	captcha := &mocks.VerificadorCaptcha{FnVerificar: func(ctx context.Context, token, accion, ip string) (float64, error) {
		return 1.0, nil
	}}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, captcha)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
		TokenCaptcha:      "token-valido",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if decision.Permitido {
		t.Fatalf("se esperaba Permitido=false por límite de IP incluso con captcha válido, obtuvo %+v", decision)
	}
	if decision.Motivo != "limite_ip_excedido" {
		t.Fatalf("motivo = %q, esperado limite_ip_excedido", decision.Motivo)
	}
	if decision.ReintentarEn != 30*time.Second {
		t.Fatalf("ReintentarEn = %v, esperado 30s", decision.ReintentarEn)
	}
	// El límite de IP no debe evaluar la cuenta (short-circuit): un
	// atacante que agote el límite de IP no necesita agotar también el de
	// cuenta para que se le bloquee.
	for _, l := range limitador.LlamadasPermitir {
		if l.Clave == "confianza:rl:cuenta:login:ana@ejemplo.com" {
			t.Fatalf("no debería haberse consultado el límite de cuenta tras bloquear por IP")
		}
	}
}

func TestEvaluar_limiteDeCuentaSinCaptcha_exigeCaptcha(t *testing.T) {
	limitador := &mocks.LimitadorTasa{
		FnPermitir: func(ctx context.Context, clave string, umbral dominio.Umbral) (bool, int, time.Duration, error) {
			if clave == "confianza:rl:cuenta:login:ana@ejemplo.com" {
				return false, 0, 5 * time.Minute, nil
			}
			return true, 5, 0, nil
		},
	}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, &mocks.VerificadorCaptcha{})

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if decision.Permitido {
		t.Fatalf("se esperaba Permitido=false, obtuvo %+v", decision)
	}
	if !decision.RequiereCaptcha {
		t.Fatalf("se esperaba RequiereCaptcha=true, obtuvo %+v", decision)
	}
	if decision.Motivo != "limite_cuenta_excedido_requiere_captcha" {
		t.Fatalf("motivo = %q", decision.Motivo)
	}
}

func TestEvaluar_limiteDeCuentaConCaptchaValido_permiteBypass(t *testing.T) {
	limitador := &mocks.LimitadorTasa{
		FnPermitir: func(ctx context.Context, clave string, umbral dominio.Umbral) (bool, int, time.Duration, error) {
			if clave == "confianza:rl:cuenta:login:ana@ejemplo.com" {
				return false, 0, 5 * time.Minute, nil
			}
			return true, 5, 0, nil
		},
	}
	captcha := &mocks.VerificadorCaptcha{FnVerificar: func(ctx context.Context, token, accion, ip string) (float64, error) {
		return 1.0, nil
	}}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, captcha)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
		TokenCaptcha:      "token-valido",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !decision.Permitido {
		t.Fatalf("un captcha válido debería levantar el cooldown de cuenta, obtuvo %+v", decision)
	}
}

func TestEvaluar_captchaPuntajeBajo_deniegaAunqueNingunLimiteSeSupere(t *testing.T) {
	captcha := &mocks.VerificadorCaptcha{FnVerificar: func(ctx context.Context, token, accion, ip string) (float64, error) {
		return 0.1, nil
	}}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(&mocks.LimitadorTasa{}, captcha)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:       dominio.AccionRegistro,
		IPOrigen:     "203.0.113.1",
		TokenCaptcha: "token-sospechoso",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if decision.Permitido {
		t.Fatalf("un puntaje de captcha bajo debe denegar aunque los límites no se hayan superado, obtuvo %+v", decision)
	}
	if decision.Motivo != "captcha_puntaje_bajo" {
		t.Fatalf("motivo = %q", decision.Motivo)
	}
}

func TestEvaluar_errorDeCaptcha_tratadoComoPuntajeCeroFailClosed(t *testing.T) {
	captcha := &mocks.VerificadorCaptcha{FnVerificar: func(ctx context.Context, token, accion, ip string) (float64, error) {
		return 0, context.DeadlineExceeded
	}}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(&mocks.LimitadorTasa{}, captcha)

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:       dominio.AccionRegistro,
		TokenCaptcha: "token-cualquiera",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if decision.Permitido {
		t.Fatalf("un fallo al verificar el captcha debe tratarse fail-closed (puntaje 0), obtuvo %+v", decision)
	}
}

func TestEvaluar_limitadorCaido_esFailOpen(t *testing.T) {
	limitador := &mocks.LimitadorTasa{FnPermitir: func(ctx context.Context, clave string, umbral dominio.Umbral) (bool, int, time.Duration, error) {
		return false, 0, 0, context.DeadlineExceeded
	}}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, &mocks.VerificadorCaptcha{})

	decision, err := caso.Evaluar(context.Background(), aplicacion.Solicitud{
		Accion:            dominio.AccionLogin,
		IPOrigen:          "203.0.113.1",
		CorreoNormalizado: "ana@ejemplo.com",
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !decision.Permitido {
		t.Fatalf("un limitador caído debe ser fail-open (Redis abajo no puede tumbar login), obtuvo %+v", decision)
	}
}

func TestRegistrarResultado_reiniciaContadorDeCuentaTrasExito(t *testing.T) {
	limitador := &mocks.LimitadorTasa{}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, &mocks.VerificadorCaptcha{})

	if err := caso.RegistrarResultado(context.Background(), aplicacion.ResultadoIntento{
		Accion:            dominio.AccionLogin,
		CorreoNormalizado: "ana@ejemplo.com",
		Exitoso:           true,
	}); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(limitador.LlamadasReiniciar) != 1 || limitador.LlamadasReiniciar[0] != "confianza:rl:cuenta:login:ana@ejemplo.com" {
		t.Fatalf("se esperaba un reinicio de confianza:rl:cuenta:login:ana@ejemplo.com, hubo %v", limitador.LlamadasReiniciar)
	}
}

func TestRegistrarResultado_noHaceNadaEnFallo(t *testing.T) {
	limitador := &mocks.LimitadorTasa{}
	caso := aplicacion.NuevoEvaluarTrustSignalCasoDeUso(limitador, &mocks.VerificadorCaptcha{})

	if err := caso.RegistrarResultado(context.Background(), aplicacion.ResultadoIntento{
		Accion:            dominio.AccionLogin,
		CorreoNormalizado: "ana@ejemplo.com",
		Exitoso:           false,
	}); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(limitador.LlamadasReiniciar) != 0 {
		t.Fatalf("un intento fallido no debe reiniciar el contador, hubo %v", limitador.LlamadasReiniciar)
	}
}
