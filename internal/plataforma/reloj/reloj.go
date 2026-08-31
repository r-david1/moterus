package reloj

import "time"

// Real implementa el puerto Reloj de cada bounded context (todos comparten
// la misma firma estructural Ahora() time.Time) devolviendo la hora real del
// sistema en UTC. No importa ningún paquete de dominio: satisface el puerto
// por duck typing, como exige la regla de que plataforma no conoce tipos de
// negocio.
type Real struct{}

// NuevoReal construye el reloj real.
func NuevoReal() Real { return Real{} }

// Ahora devuelve la hora actual en UTC.
func (Real) Ahora() time.Time { return time.Now().UTC() }

// Fija es un reloj controlable para tests de integración: siempre devuelve
// el mismo instante hasta que se le asigna uno nuevo con Fijar.
type Fija struct {
	instante time.Time
}

// NuevaFija construye un reloj fijo con el instante indicado.
func NuevaFija(instante time.Time) *Fija {
	return &Fija{instante: instante}
}

// Ahora devuelve el instante fijado.
func (f *Fija) Ahora() time.Time { return f.instante }

// Fijar cambia el instante devuelto por Ahora.
func (f *Fija) Fijar(instante time.Time) { f.instante = instante }
