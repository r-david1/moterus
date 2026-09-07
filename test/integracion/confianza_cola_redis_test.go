package integracion

import (
	"context"
	"os"
	"testing"
	"time"

	confianzacripto "github.com/r-david1/moterus/internal/confianza/adaptadores/cripto"
	confianzaredis "github.com/r-david1/moterus/internal/confianza/adaptadores/redis"
	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
	"github.com/r-david1/moterus/internal/plataforma/cache"
	"github.com/r-david1/moterus/internal/plataforma/ids"
)

// Este archivo implementa el test de consistencia dominio↔Lua que exige
// §1.5 del diseño colas-virtuales.md ("sin ese test, las dos copias se
// desincronizan en silencio"): ejecuta los scripts Lua reales de
// confianza/adaptadores/redis.EstadoCola contra un Redis real y compara sus
// resultados, caso por caso, con dominio.SalaDeEspera.CursorEn/TurnoDe y
// dominio.TicketDeCola.Desenlace para los MISMOS valores. No es un test de
// dominio con mocks (esos ya existen en internal/confianza/dominio) ni un
// test de humo del adaptador: es específicamente el punto de comparación
// entre las dos copias de la aritmética.
//
// Requiere Redis real (mismo criterio que confianza_redis_test.go): sin
// REDIS_URL, se salta limpiamente.

// estadoColaRedis abre un EstadoCola contra REDIS_URL, con el reloj interno
// reemplazado por relojFn (ConRelojEstadoCola): así el test controla
// exactamente qué "ahora_ms" ve el script Lua, en vez de depender de dos
// llamadas independientes a time.Now() separadas por latencia de red.
func estadoColaRedis(t *testing.T, relojFn func() time.Time) *confianzaredis.EstadoCola {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL no está definido: se omiten los tests de integración de Confianza/Redis (colas de acceso virtual)")
	}
	cliente, err := cache.NuevoClienteRedis(url)
	if err != nil {
		t.Fatalf("no se pudo construir el cliente Redis: %v", err)
	}
	if err := cliente.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("no se pudo conectar a Redis en %s: %v", url, err)
	}
	t.Cleanup(func() { _ = cliente.Close() })
	return confianzaredis.NuevoEstadoCola(cliente, confianzaredis.ConRelojEstadoCola(relojFn))
}

// salaDeEsperaDePrueba construye, vía dominio.ReconstituirSalaDeEspera
// (rehidratación: no valida invariantes de negocio, así que puede fijar
// libremente cursorBase/relojDesde para el corpus de prueba), una sala
// "abierta" con alcance organizacion y un IDOrganizacion aleatorio nuevo:
// eso le da a cada caso una ClaveSala ("org:<uuid>:<ruta>") única, sin
// colisionar con los de otros casos ni con los del resto de la suite —
// RutaProtegida es un catálogo cerrado de solo 3 valores y no alcanza para
// dar unicidad por sí sola.
func salaDeEsperaDePrueba(t *testing.T, ruta dominio.RutaProtegida, cursorBase int64, relojDesde time.Time, ritmoPorSegundo int, ventanaReclamo time.Duration) *dominio.SalaDeEspera {
	t.Helper()
	idCrudo, err := ids.GenerarUUIDv7()
	if err != nil {
		t.Fatalf("GenerarUUIDv7 (sala): %v", err)
	}
	id, err := dominio.IDSalaDeEsperaDesde(idCrudo)
	if err != nil {
		t.Fatalf("IDSalaDeEsperaDesde: %v", err)
	}
	orgCrudo, err := ids.GenerarUUIDv7()
	if err != nil {
		t.Fatalf("GenerarUUIDv7 (organizacion): %v", err)
	}
	orgID, err := dominio.IDOrganizacionDesde(orgCrudo)
	if err != nil {
		t.Fatalf("IDOrganizacionDesde: %v", err)
	}
	alcance, err := dominio.AlcanceOrganizacion(orgID)
	if err != nil {
		t.Fatalf("AlcanceOrganizacion: %v", err)
	}
	alias, err := dominio.NuevoAliasSala("prueba-consistencia-cola")
	if err != nil {
		t.Fatalf("NuevoAliasSala: %v", err)
	}
	ritmo, err := dominio.NuevoRitmoAdmision(ritmoPorSegundo)
	if err != nil {
		t.Fatalf("NuevoRitmoAdmision(%d): %v", ritmoPorSegundo, err)
	}
	politica, err := dominio.NuevaPoliticaSala(ritmo, 5_000_000, ventanaReclamo, dominio.ModoDegradadoPermitir)
	if err != nil {
		t.Fatalf("NuevaPoliticaSala: %v", err)
	}
	return dominio.ReconstituirSalaDeEspera(
		id, alias, alcance, ruta, dominio.EstadoSalaAbierta, politica,
		cursorBase, relojDesde, nil, relojDesde, nil, nil,
	)
}

// proyectarSalaDePrueba escribe la configuración de sala en Redis
// (EstadoDeCola.Proyectar) y registra su limpieza al terminar el test
// (Retirar: borra cfg, seq y todos los tickets de esa clave).
func proyectarSalaDePrueba(t *testing.T, ctx context.Context, estadoCola *confianzaredis.EstadoCola, sala *dominio.SalaDeEspera) string {
	t.Helper()
	clave := sala.Clave().String()
	proy := puertos.ProyeccionSala{
		Clave:               clave,
		Alias:               sala.Alias().Normalizado(),
		Estado:              sala.Estado().String(),
		RitmoAdmision:       sala.Politica().RitmoAdmision().PorSegundo(),
		CapacidadMaximaCola: sala.Politica().CapacidadMaximaCola(),
		VentanaReclamo:      sala.Politica().VentanaReclamo(),
		CursorBase:          sala.CursorBase(),
		RelojDesde:          sala.RelojDesde(),
		Version:             1,
	}
	if err := estadoCola.Proyectar(ctx, proy); err != nil {
		t.Fatalf("Proyectar: %v", err)
	}
	t.Cleanup(func() { _ = estadoCola.Retirar(context.Background(), clave) })
	return clave
}

// TestEstadoColaRedis_ConsistenciaCursorYTurnoConDominio es el test de
// consistencia central de §1.5 del diseño: para cada caso, proyecta una
// sala con un (cursorBase, relojDesde, ritmo) dado, ingresa varios tickets
// con el reloj del adaptador fijado exactamente en "ahora", y compara el
// Cursor/TurnoEstimadoEn que devuelve el script Lua `ingresar` —para cada
// rango sucesivo que va asignando INCR— contra
// dominio.SalaDeEspera.CursorEn(ahora) y
// dominio.SalaDeEspera.TurnoDe(rango) calculados con los MISMOS valores.
//
// El corpus incluye deliberadamente casos donde rango <= cursorBase (turno
// ya alcanzado desde el propio ancla, delta <= 0 en la fórmula de
// TurnoDe): es exactamente el caso donde el script `reclamar` tal como lo
// trae el diseño, tomado literal, se desviaba del dominio (ver el
// comentario de funcionesLuaCursorTurno en estado_cola.go).
func TestEstadoColaRedis_ConsistenciaCursorYTurnoConDominio(t *testing.T) {
	referencia := time.Now().UTC().Truncate(time.Millisecond)

	casos := []struct {
		nombre           string
		ruta             dominio.RutaProtegida
		cursorBase       int64
		relojDesdeOffset time.Duration
		ritmoPorSegundo  int
		ahoraOffset      time.Duration
		ingresos         int
	}{
		{"ahora_igual_a_relojDesde", dominio.RutaAccesoIniciarSesion, 0, 0, 50, 0, 3},
		{"ahora_avanza_dos_segundos", dominio.RutaAccesoIniciarSesion, 0, 0, 50, 2 * time.Second, 3},
		{"cursorBase_no_cero_ritmo_alto", dominio.RutaIdentidadRegistrarUsuario, 1000, 0, 200, 3500 * time.Millisecond, 5},
		{"relojDesde_en_el_futuro_se_recorta_a_cero", dominio.RutaTenenciaAceptarInvitacion, 500, 10 * time.Second, 10, 0, 2},
		{"ritmo_maximo_offset_no_multiplo", dominio.RutaAccesoIniciarSesion, 0, -100 * time.Millisecond, 10000, 999 * time.Millisecond, 4},
		{"ritmo_minimo_division_no_exacta", dominio.RutaIdentidadRegistrarUsuario, 123456, 0, 1, 59999 * time.Millisecond, 3},
		{"rango_por_debajo_de_cursorBase_delta_no_positivo", dominio.RutaTenenciaAceptarInvitacion, 10, 0, 5, 500 * time.Millisecond, 3},
		{"offset_negativo_grande_ritmo_bajo", dominio.RutaAccesoIniciarSesion, 0, 0, 3, -5 * time.Second, 2},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			relojDesde := referencia.Add(caso.relojDesdeOffset)
			ahoraFijo := referencia.Add(caso.ahoraOffset)

			sala := salaDeEsperaDePrueba(t, caso.ruta, caso.cursorBase, relojDesde, caso.ritmoPorSegundo, 2*time.Minute)
			ctx := context.Background()
			estadoCola := estadoColaRedis(t, func() time.Time { return ahoraFijo })
			clave := proyectarSalaDePrueba(t, ctx, estadoCola, sala)

			cursorEsperado := sala.CursorEn(ahoraFijo)

			generador := confianzacripto.NuevoGeneradorTickets()
			for i := 1; i <= caso.ingresos; i++ {
				plano, err := generador.GenerarTicket()
				if err != nil {
					t.Fatalf("GenerarTicket: %v", err)
				}
				hash := plano.Hash()

				resultado, err := estadoCola.Ingresar(ctx, clave, hash.Valor())
				if err != nil {
					t.Fatalf("Ingresar (rango %d): %v", i, err)
				}
				if resultado.Desenlace != dominio.DesenlaceEsperando.String() {
					t.Fatalf("Ingresar (rango %d): desenlace = %q, esperado %q", i, resultado.Desenlace, dominio.DesenlaceEsperando.String())
				}
				if resultado.Rango != int64(i) {
					t.Fatalf("Ingresar: rango asignado = %d, esperado %d (INCR debe ser secuencial)", resultado.Rango, i)
				}
				if resultado.Cursor != cursorEsperado {
					t.Errorf("Cursor (rango %d): Lua = %d, dominio.CursorEn = %d", i, resultado.Cursor, cursorEsperado)
				}

				rango, err := dominio.NuevoRango(int64(i))
				if err != nil {
					t.Fatalf("NuevoRango(%d): %v", i, err)
				}
				turnoEsperado := sala.TurnoDe(rango)
				if resultado.TurnoEstimadoEn.UnixMilli() != turnoEsperado.UnixMilli() {
					t.Errorf("TurnoEstimadoEn (rango %d): Lua = %v (%d ms), dominio.TurnoDe = %v (%d ms)",
						i, resultado.TurnoEstimadoEn, resultado.TurnoEstimadoEn.UnixMilli(),
						turnoEsperado, turnoEsperado.UnixMilli())
				}
			}
		})
	}
}

// TestEstadoColaRedis_ConsistenciaDesenlaceReclamoConDominio compara el
// desenlace que produce el script Lua `reclamar` contra
// dominio.TicketDeCola.Desenlace para los cuatro desenlaces que ese método
// de dominio puede producir (admitido, esperando, turno_caducado,
// ticket_consumido — ver el comentario de TicketDeCola.Desenlace: los otros
// tres valores del catálogo, ticket_desconocido/sala_cerrada/cola_llena,
// ocurren antes de poder construir un TicketDeCola en memoria).
func TestEstadoColaRedis_ConsistenciaDesenlaceReclamoConDominio(t *testing.T) {
	referencia := time.Now().UTC().Truncate(time.Millisecond)
	ctx := context.Background()

	casos := []struct {
		nombre           string
		ruta             dominio.RutaProtegida
		ritmoPorSegundo  int
		ventanaReclamo   time.Duration
		ahoraIngreso     time.Duration // offset desde referencia
		ahoraReclamo     time.Duration // offset desde referencia
		reclamarDosVeces bool
	}{
		{"admitido_turno_ya_alcanzado", dominio.RutaAccesoIniciarSesion, 1000, 2 * time.Minute, 0, 10 * time.Second, false},
		{"esperando_turno_todavia_no_llega", dominio.RutaIdentidadRegistrarUsuario, 1, 2 * time.Minute, 0, 0, false},
		{"turno_caducado_fuera_de_la_ventana", dominio.RutaTenenciaAceptarInvitacion, 1000, 30 * time.Second, 0, 40 * time.Second, false},
		{"ticket_consumido_segundo_reclamo", dominio.RutaAccesoIniciarSesion, 1000, 2 * time.Minute, 0, 10 * time.Second, true},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			relojDesde := referencia
			sala := salaDeEsperaDePrueba(t, caso.ruta, 0, relojDesde, caso.ritmoPorSegundo, caso.ventanaReclamo)

			var ahoraActual time.Time
			estadoCola := estadoColaRedis(t, func() time.Time { return ahoraActual })
			clave := proyectarSalaDePrueba(t, ctx, estadoCola, sala)

			// Ingreso: fija el reloj en ahoraIngreso.
			ahoraActual = referencia.Add(caso.ahoraIngreso)
			generador := confianzacripto.NuevoGeneradorTickets()
			plano, err := generador.GenerarTicket()
			if err != nil {
				t.Fatalf("GenerarTicket: %v", err)
			}
			hash := plano.Hash()
			ingreso, err := estadoCola.Ingresar(ctx, clave, hash.Valor())
			if err != nil {
				t.Fatalf("Ingresar: %v", err)
			}
			rango, err := dominio.NuevoRango(ingreso.Rango)
			if err != nil {
				t.Fatalf("NuevoRango(%d): %v", ingreso.Rango, err)
			}

			// Reclamo(s): fija el reloj en ahoraReclamo.
			ahoraActual = referencia.Add(caso.ahoraReclamo)

			estadoTicketDominio := dominio.EstadoTicketEsperando
			if caso.reclamarDosVeces {
				// El primer reclamo debe admitir y consumir el ticket; el
				// segundo es el que se compara contra "ticket_consumido".
				primero, err := estadoCola.Reclamar(ctx, clave, hash.Valor())
				if err != nil {
					t.Fatalf("primer reclamo: %v", err)
				}
				if primero.Desenlace != dominio.DesenlaceAdmitido.String() {
					t.Fatalf("primer reclamo: desenlace = %q, esperado admitido", primero.Desenlace)
				}
				estadoTicketDominio = dominio.EstadoTicketConsumido
			}

			resultado, err := estadoCola.Reclamar(ctx, clave, hash.Valor())
			// Un desenlace que no sea "admitido" viaja como error de
			// dominio en la capa de aplicación (ver
			// aplicacion.errorDesdeDesenlaceReclamo), pero el puerto
			// EstadoDeCola en sí no distingue: Reclamar devuelve
			// (EstadoTicket, error) donde error solo es no-nil ante un
			// fallo de infraestructura real. Acá se ignora `err` a
			// propósito: lo que importa es `resultado.Desenlace`.
			_ = err

			ticket, errTicket := dominio.NuevoTicketDeCola(hash, sala.Clave(), rango, estadoTicketDominio, referencia.Add(caso.ahoraIngreso))
			if errTicket != nil {
				t.Fatalf("NuevoTicketDeCola: %v", errTicket)
			}
			desenlaceEsperado := ticket.Desenlace(sala, ahoraActual)

			if resultado.Desenlace != desenlaceEsperado.String() {
				t.Errorf("Desenlace del reclamo: Lua = %q, dominio.TicketDeCola.Desenlace = %q", resultado.Desenlace, desenlaceEsperado.String())
			}
		})
	}
}
