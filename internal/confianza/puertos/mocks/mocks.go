// Package mocks contiene test doubles escritos a mano para los puertos de
// salida de Confianza (puertos/salida.go), siguiendo el mismo patrón que
// identidad/puertos/mocks: un struct por interfaz con un campo FnX
// opcional por método y un comportamiento por defecto razonable cuando no
// se configura.
package mocks

import (
	"context"
	"time"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// --- LimitadorTasa -----------------------------------------------------------

// LimitadorTasa es el test double de puertos.LimitadorTasa. Por defecto
// (FnPermitir sin configurar) siempre permite, sin restantes ni backoff:
// el camino feliz más común en los tests del caso de uso.
type LimitadorTasa struct {
	FnPermitir  func(ctx context.Context, clave string, umbral dominio.Umbral) (bool, int, time.Duration, error)
	FnReiniciar func(ctx context.Context, clave string) error

	LlamadasPermitir  []LlamadaPermitir
	LlamadasReiniciar []string
}

// LlamadaPermitir captura los argumentos de una llamada a Permitir.
type LlamadaPermitir struct {
	Clave  string
	Umbral dominio.Umbral
}

var _ puertos.LimitadorTasa = (*LimitadorTasa)(nil)

func (m *LimitadorTasa) Permitir(ctx context.Context, clave string, umbral dominio.Umbral) (bool, int, time.Duration, error) {
	m.LlamadasPermitir = append(m.LlamadasPermitir, LlamadaPermitir{Clave: clave, Umbral: umbral})
	if m.FnPermitir != nil {
		return m.FnPermitir(ctx, clave, umbral)
	}
	return true, umbral.Limite, 0, nil
}

func (m *LimitadorTasa) Reiniciar(ctx context.Context, clave string) error {
	m.LlamadasReiniciar = append(m.LlamadasReiniciar, clave)
	if m.FnReiniciar != nil {
		return m.FnReiniciar(ctx, clave)
	}
	return nil
}

// --- VerificadorCaptcha --------------------------------------------------

// VerificadorCaptcha es el test double de puertos.VerificadorCaptcha. Por
// defecto (FnVerificar sin configurar) devuelve puntaje 1.0 sin error.
type VerificadorCaptcha struct {
	FnVerificar func(ctx context.Context, token, accion, ip string) (float64, error)

	LlamadasVerificar []LlamadaVerificar
}

// LlamadaVerificar captura los argumentos de una llamada a Verificar.
type LlamadaVerificar struct {
	Token  string
	Accion string
	IP     string
}

var _ puertos.VerificadorCaptcha = (*VerificadorCaptcha)(nil)

func (m *VerificadorCaptcha) Verificar(ctx context.Context, token, accion, ip string) (float64, error) {
	m.LlamadasVerificar = append(m.LlamadasVerificar, LlamadaVerificar{Token: token, Accion: accion, IP: ip})
	if m.FnVerificar != nil {
		return m.FnVerificar(ctx, token, accion, ip)
	}
	return 1.0, nil
}
