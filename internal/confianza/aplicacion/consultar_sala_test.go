package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/confianza/aplicacion"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/confianza/puertos/mocks"
)

func TestConsultarSalaCasoDeUso_ObtenerPorAlias_FlujoFeliz(t *testing.T) {
	ahora := ahoraDePrueba()
	sala := salaAbiertaDePrueba(t, idSalaValido1, 50, ahora)
	instantanea := instantaneaConSala(t, sala)
	estadoCola := &mocks.EstadoDeCola{
		FnInstantanea: func(ctx context.Context, clave string) (puertos.InstantaneaCola, error) {
			return puertos.InstantaneaCola{LongitudAproximada: 500, Cursor: 10, Ingresos: 510}, nil
		},
	}
	consultor := aplicacion.NuevoConsultarSalaCasoDeUso(instantanea, estadoCola)

	vista, err := consultor.ObtenerPorAlias(context.Background(), puertos.ConsultaSalaPorAlias{Alias: "evento-" + idSalaValido1})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if vista.Estado != dominio.EstadoSalaAbierta.String() {
		t.Errorf("Estado = %q, esperado abierta", vista.Estado)
	}
	if vista.LongitudAproximada != 500 {
		t.Errorf("LongitudAproximada = %d, esperado 500", vista.LongitudAproximada)
	}
	if esperado := 10 * time.Second; vista.EsperaEstimada != esperado {
		t.Errorf("EsperaEstimada = %v, esperado %v (500 turnos a 50/s)", vista.EsperaEstimada, esperado)
	}
	if vista.ReconsultarEn <= 0 {
		t.Error("ReconsultarEn debía ser positivo")
	}
}

func TestConsultarSalaCasoDeUso_ObtenerPorAlias_SalaNoEncontrada(t *testing.T) {
	consultor := aplicacion.NuevoConsultarSalaCasoDeUso(aplicacion.NuevaInstantaneaSalasVigentes(), &mocks.EstadoDeCola{})

	_, err := consultor.ObtenerPorAlias(context.Background(), puertos.ConsultaSalaPorAlias{Alias: "no-existe"})
	var errNoEncontrada *dominio.ErrSalaNoEncontrada
	if !errors.As(err, &errNoEncontrada) {
		t.Fatalf("se esperaba *ErrSalaNoEncontrada, obtuvo %T: %v", err, err)
	}
}
