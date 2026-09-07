package integracion

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	confianzapostgres "github.com/r-david1/moterus/internal/confianza/adaptadores/postgres"
	"github.com/r-david1/moterus/internal/confianza/dominio"
)

// unicoDePrueba genera un AliasSala válido (minúsculas, [a-z0-9-], 3-48
// caracteres) y único por corrida, para no colisionar con el índice único
// salas_espera_alias_idx entre ejecuciones repetidas de la suite contra el
// mismo Postgres compartido de desarrollo.
func unicoDePrueba(t *testing.T, prefijo string) string {
	t.Helper()
	sufijo := strconv.FormatInt(time.Now().UnixNano(), 36)
	valor := prefijo + "-" + sufijo
	if len(valor) > 48 {
		valor = valor[:48]
	}
	return valor
}

// Este archivo ejercita confianza/adaptadores/postgres.RepositorioSalasDeEspera
// y GeneradorIDs contra Postgres real (mismo criterio que el resto de
// test/integracion: sin mocks, con la pila real incluyendo los índices
// únicos de la migración 000017_crear_salas_espera). Usa poolAplicacion
// (rol_login_identidad, que hereda de rol_aplicacion) — los mismos
// privilegios acotados con los que corre el proceso api en producción
// (ADR 0017): si el GRANT de 000017 estuviera mal, este test lo detecta.

// cerrarAlFinalizar registra un t.Cleanup que transiciona sala a "cerrada"
// y la persiste: salas_espera_vigente_idx (INV-COLA-01) admite a lo sumo
// una fila NO cerrada por (alcance, ruta), y solo hay 3 rutas en el
// catálogo cerrado — sin este cleanup, una segunda corrida de esta suite
// contra el mismo Postgres compartido de desarrollo (o un segundo test que
// reutilice la misma ruta) chocaría con la sala que dejó abierta una
// corrida anterior. No hay DELETE disponible para rol_aplicacion (000017:
// una sala cerrada es evidencia de un evento operativo), así que "cerrar"
// —no borrar— es la única limpieza posible, y es exactamente lo que haría
// un operador real.
func cerrarAlFinalizar(t *testing.T, repo *confianzapostgres.RepositorioSalasDeEspera, sala *dominio.SalaDeEspera) {
	t.Helper()
	t.Cleanup(func() {
		if sala.Estado().EsIgual(dominio.EstadoSalaCerrada) {
			return
		}
		if err := sala.Cerrar(0, 0, time.Now().UTC()); err != nil {
			return
		}
		_ = repo.Guardar(context.Background(), sala)
	})
}

func nuevaSalaAbiertaDePrueba(t *testing.T, ids confianzapostgres.GeneradorIDs, alias string, ruta dominio.RutaProtegida, ahora time.Time) *dominio.SalaDeEspera {
	t.Helper()
	id, err := ids.NuevoIDSalaDeEspera()
	if err != nil {
		t.Fatalf("NuevoIDSalaDeEspera: %v", err)
	}
	aliasVO, err := dominio.NuevoAliasSala(alias)
	if err != nil {
		t.Fatalf("NuevoAliasSala(%q): %v", alias, err)
	}
	ritmo, err := dominio.NuevoRitmoAdmision(50)
	if err != nil {
		t.Fatalf("NuevoRitmoAdmision: %v", err)
	}
	politica, err := dominio.NuevaPoliticaSala(ritmo, 500_000, 2*time.Minute, dominio.ModoDegradadoPermitir)
	if err != nil {
		t.Fatalf("NuevaPoliticaSala: %v", err)
	}
	sala, err := dominio.NuevaSalaDeEspera(id, aliasVO, dominio.AlcanceSistema(), ruta, politica, nil, ahora)
	if err != nil {
		t.Fatalf("NuevaSalaDeEspera: %v", err)
	}
	if err := sala.Abrir(ahora); err != nil {
		t.Fatalf("Abrir: %v", err)
	}
	return sala
}

func TestRepositorioSalasDeEsperaPostgres_GuardarYBuscarPorID(t *testing.T) {
	pool := poolAplicacion(t)
	repo := confianzapostgres.NuevoRepositorioSalasDeEspera(pool)
	gen := confianzapostgres.NuevoGeneradorIDs()
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Millisecond)

	sala := nuevaSalaAbiertaDePrueba(t, gen, unicoDePrueba(t, "sala-postgres"), dominio.RutaAccesoIniciarSesion, ahora)

	if err := repo.Guardar(ctx, sala); err != nil {
		t.Fatalf("Guardar (insert): %v", err)
	}
	cerrarAlFinalizar(t, repo, sala)

	recuperada, err := repo.BuscarPorID(ctx, sala.ID())
	if err != nil {
		t.Fatalf("BuscarPorID: %v", err)
	}
	if recuperada == nil {
		t.Fatal("BuscarPorID devolvió nil para una sala recién guardada")
	}
	if !recuperada.ID().EsIgual(sala.ID()) {
		t.Errorf("ID recuperado = %v, esperado %v", recuperada.ID(), sala.ID())
	}
	if !recuperada.Alias().EsIgual(sala.Alias()) {
		t.Errorf("Alias recuperado = %v, esperado %v", recuperada.Alias(), sala.Alias())
	}
	if !recuperada.Estado().EsIgual(dominio.EstadoSalaAbierta) {
		t.Errorf("Estado recuperado = %v, esperado abierta", recuperada.Estado())
	}
	if recuperada.Politica().RitmoAdmision().PorSegundo() != 50 {
		t.Errorf("RitmoAdmision recuperado = %d, esperado 50", recuperada.Politica().RitmoAdmision().PorSegundo())
	}
	abiertaEn, ok := recuperada.AbiertaEn()
	if !ok {
		t.Fatal("AbiertaEn: se esperaba un valor tras Abrir")
	}
	if !abiertaEn.Equal(ahora) {
		t.Errorf("AbiertaEn recuperado = %v, esperado %v", abiertaEn, ahora)
	}

	// Guardar de nuevo (mutación de negocio: CambiarRitmo) ejercita la rama
	// UPDATE del upsert (ON CONFLICT (id) DO UPDATE).
	nuevoRitmo, err := dominio.NuevoRitmoAdmision(120)
	if err != nil {
		t.Fatalf("NuevoRitmoAdmision(120): %v", err)
	}
	if err := recuperada.CambiarRitmo(nuevoRitmo, 0, ahora.Add(time.Minute)); err != nil {
		t.Fatalf("CambiarRitmo: %v", err)
	}
	if err := repo.Guardar(ctx, recuperada); err != nil {
		t.Fatalf("Guardar (update): %v", err)
	}
	trasActualizar, err := repo.BuscarPorID(ctx, sala.ID())
	if err != nil {
		t.Fatalf("BuscarPorID tras actualizar: %v", err)
	}
	if trasActualizar.Politica().RitmoAdmision().PorSegundo() != 120 {
		t.Errorf("RitmoAdmision tras actualizar = %d, esperado 120", trasActualizar.Politica().RitmoAdmision().PorSegundo())
	}
}

func TestRepositorioSalasDeEsperaPostgres_BuscarPorAliasYPorIDInexistenteDevuelveNil(t *testing.T) {
	pool := poolAplicacion(t)
	repo := confianzapostgres.NuevoRepositorioSalasDeEspera(pool)
	ctx := context.Background()

	aliasInexistente, err := dominio.NuevoAliasSala(unicoDePrueba(t, "no-existe"))
	if err != nil {
		t.Fatalf("NuevoAliasSala: %v", err)
	}
	sala, err := repo.BuscarPorAlias(ctx, aliasInexistente)
	if err != nil {
		t.Fatalf("BuscarPorAlias: %v", err)
	}
	if sala != nil {
		t.Errorf("BuscarPorAlias(%q) = %v, esperado nil", aliasInexistente, sala)
	}

	gen := confianzapostgres.NuevoGeneradorIDs()
	idInexistente, err := gen.NuevoIDSalaDeEspera()
	if err != nil {
		t.Fatalf("NuevoIDSalaDeEspera: %v", err)
	}
	salaPorID, err := repo.BuscarPorID(ctx, idInexistente)
	if err != nil {
		t.Fatalf("BuscarPorID: %v", err)
	}
	if salaPorID != nil {
		t.Errorf("BuscarPorID(%v) = %v, esperado nil", idInexistente, salaPorID)
	}
}

// TestRepositorioSalasDeEsperaPostgres_ListarVigentes verifica que
// ListarVigentes (la consulta del reconciliador, §3.7 del diseño) devuelve
// una sala recién abierta y no una recién cerrada.
func TestRepositorioSalasDeEsperaPostgres_ListarVigentes(t *testing.T) {
	pool := poolAplicacion(t)
	repo := confianzapostgres.NuevoRepositorioSalasDeEspera(pool)
	gen := confianzapostgres.NuevoGeneradorIDs()
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Millisecond)

	vigente := nuevaSalaAbiertaDePrueba(t, gen, unicoDePrueba(t, "vigente"), dominio.RutaIdentidadRegistrarUsuario, ahora)
	if err := repo.Guardar(ctx, vigente); err != nil {
		t.Fatalf("Guardar (vigente): %v", err)
	}
	cerrarAlFinalizar(t, repo, vigente)

	cerrada := nuevaSalaAbiertaDePrueba(t, gen, unicoDePrueba(t, "cerrada"), dominio.RutaTenenciaAceptarInvitacion, ahora)
	if err := cerrada.Cerrar(0, 0, ahora.Add(time.Second)); err != nil {
		t.Fatalf("Cerrar: %v", err)
	}
	if err := repo.Guardar(ctx, cerrada); err != nil {
		t.Fatalf("Guardar (cerrada): %v", err)
	}

	vigentes, err := repo.ListarVigentes(ctx)
	if err != nil {
		t.Fatalf("ListarVigentes: %v", err)
	}
	var encontroVigente, encontroCerrada bool
	for _, s := range vigentes {
		if s.ID().EsIgual(vigente.ID()) {
			encontroVigente = true
		}
		if s.ID().EsIgual(cerrada.ID()) {
			encontroCerrada = true
		}
	}
	if !encontroVigente {
		t.Error("ListarVigentes no incluyó la sala abierta")
	}
	if encontroCerrada {
		t.Error("ListarVigentes incluyó una sala cerrada")
	}
}

// TestRepositorioSalasDeEsperaPostgres_AliasDuplicadoYSalaYaAbierta verifica
// la traducción de los dos índices únicos de la migración 000017 (§6.1 del
// diseño, INV-COLA-01) a los errores de dominio tipados correspondientes.
func TestRepositorioSalasDeEsperaPostgres_AliasDuplicadoYSalaYaAbierta(t *testing.T) {
	pool := poolAplicacion(t)
	repo := confianzapostgres.NuevoRepositorioSalasDeEspera(pool)
	gen := confianzapostgres.NuevoGeneradorIDs()
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Millisecond)

	alias := unicoDePrueba(t, "alias-duplicado")
	primera := nuevaSalaAbiertaDePrueba(t, gen, alias, dominio.RutaAccesoIniciarSesion, ahora)
	if err := repo.Guardar(ctx, primera); err != nil {
		t.Fatalf("Guardar (primera): %v", err)
	}
	cerrarAlFinalizar(t, repo, primera)

	// Mismo alias, ID distinto: viola salas_espera_alias_idx.
	id2, err := gen.NuevoIDSalaDeEspera()
	if err != nil {
		t.Fatalf("NuevoIDSalaDeEspera: %v", err)
	}
	aliasVO, _ := dominio.NuevoAliasSala(alias)
	ritmo, _ := dominio.NuevoRitmoAdmision(50)
	politica, _ := dominio.NuevaPoliticaSala(ritmo, 500_000, 2*time.Minute, dominio.ModoDegradadoPermitir)
	// Distinta ruta para no confundir con la violación de sala vigente que
	// se prueba más abajo.
	duplicadaAlias, err := dominio.NuevaSalaDeEspera(id2, aliasVO, dominio.AlcanceSistema(), dominio.RutaIdentidadRegistrarUsuario, politica, nil, ahora)
	if err != nil {
		t.Fatalf("NuevaSalaDeEspera (alias duplicado): %v", err)
	}
	err = repo.Guardar(ctx, duplicadaAlias)
	var errAlias *dominio.ErrAliasSalaYaRegistrado
	if !errors.As(err, &errAlias) {
		t.Fatalf("Guardar con alias duplicado: err = %v, esperado *dominio.ErrAliasSalaYaRegistrado", err)
	}

	// Misma (alcance, ruta) que la primera, distinto alias y distinto ID:
	// viola salas_espera_vigente_idx (INV-COLA-01).
	segunda := nuevaSalaAbiertaDePrueba(t, gen, unicoDePrueba(t, "otra-sala-misma-ruta"), dominio.RutaAccesoIniciarSesion, ahora)
	err = repo.Guardar(ctx, segunda)
	var errVigente *dominio.ErrSalaYaAbiertaParaLaRuta
	if !errors.As(err, &errVigente) {
		t.Fatalf("Guardar con sala ya abierta para la ruta: err = %v, esperado *dominio.ErrSalaYaAbiertaParaLaRuta", err)
	}
}
