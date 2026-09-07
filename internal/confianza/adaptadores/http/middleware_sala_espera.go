package http

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/r-david1/moterus/internal/confianza/dominio"
	"github.com/r-david1/moterus/internal/confianza/puertos"
)

// claveIDOrganizacionAutorizada es la clave no exportada bajo la que se
// publica, en el context.Context, el {idOrganizacion} que el middleware de
// autorización de un contexto consumidor (Acceso, Identidad o Tenencia) ya
// autorizó ANTES de llegar a MiddlewareSalaDeEspera (§1.1 y §7.3 del diseño
// docs/design/colas-virtuales.md).
type claveIDOrganizacionAutorizada struct{}

// ConIDOrganizacionAutorizada publica el {idOrganizacion} ya autorizado en
// ctx. Es el punto de extensión para una futura ruta org-scoped protegible
// (§1.1: ninguna de las tres rutas del catálogo cerrado actual lo es): la
// firma de MiddlewareSalaDeEspera está fijada por el diseño
// (func(huma.API, puertos.PorteroDeSala, dominio.RutaProtegida) ...) y no
// admite un parámetro adicional, así que el {idOrganizacion} tiene que
// viajar por ctx, publicado por quien ya lo autorizó (INV-COLA-09: "después
// de autorizar: la clave sale del {idOrganizacion} ya autorizado"). Hoy
// ningún consumidor lo invoca (§12 del diseño, montaje en Acceso/Identidad/
// Tenencia, fuera del alcance de esta extensión) y MiddlewareSalaDeEspera
// resuelve siempre a alcance sistema ("").
func ConIDOrganizacionAutorizada(ctx context.Context, idOrganizacion string) context.Context {
	return context.WithValue(ctx, claveIDOrganizacionAutorizada{}, idOrganizacion)
}

// idOrganizacionAutorizadaDesdeContexto recupera el valor publicado por
// ConIDOrganizacionAutorizada, o "" (alcance sistema) si nadie lo publicó.
func idOrganizacionAutorizadaDesdeContexto(ctx context.Context) string {
	id, _ := ctx.Value(claveIDOrganizacionAutorizada{}).(string)
	return id
}

// cuerpoBloqueoSala es el cuerpo RFC 9457 exacto de §7.4 del diseño: un
// ErrorModel de Huma no alcanza (no tiene lugar para desenlace/alias_sala/
// posicion/ingreso/estado_turno), así que este middleware construye y
// serializa su propio cuerpo en vez de pasar por huma.WriteErr — usando la
// misma infraestructura de negociación de contenido que el resto de la API
// (api.Negotiate/api.Marshal), para que un cliente que pida CBOR también lo
// reciba en CBOR.
type cuerpoBloqueoSala struct {
	Type                   string `json:"type"`
	Title                  string `json:"title"`
	Status                 int    `json:"status"`
	Detail                 string `json:"detail"`
	Desenlace              string `json:"desenlace"`
	AliasSala              string `json:"alias_sala"`
	Posicion               int64  `json:"posicion,omitempty"`
	EsperaEstimadaSegundos int64  `json:"espera_estimada_segundos,omitempty"`
	Ingreso                string `json:"ingreso"`
	EstadoTurno            string `json:"estado_turno"`
}

const tipoErrorSalaDeEspera = "https://moterus.dev/errores/sala-de-espera"
const tituloSalaDeEspera = "El servicio está en sala de espera"
const detalleSalaDeEspera = "Hay una cola de acceso activa para esta operación. Obtené un turno y volvé a intentar cuando sea el tuyo."

// Retry-After para los dos desenlaces sin un puertos.ResultadoTurno del que
// derivarlo (§7.4 del diseño no fija un número para estos casos; decisión
// documentada aquí):
//   - "ticket_requerido": el cliente ni siquiera presentó un ticket, no hay
//     ETA que ofrecer; 5s alcanza para que vaya a pedir uno.
//   - "sala_no_disponible": Redis no responde con modoDegradado=rechazar
//     (§8 del diseño); el propio diseño da el número exacto ("Retry-After
//     corto, 30s").
const retryTicketRequeridoSegundos = 5
const retrySalaNoDisponibleSegundos = 30

// retryDesenlaceGenericoSegundos es el piso usado cuando
// ResultadoTurno.ReconsultarEn no aporta un valor útil (cero o negativo)
// para un desenlace distinto de "esperando" (turno_caducado,
// ticket_desconocido, ticket_consumido, sala_cerrada, cola_llena): el
// cliente debe reingresar, no solo reintentar la misma petición, así que un
// valor corto es preferible a uno largo.
const retryDesenlaceGenericoSegundos = 5

// MiddlewareSalaDeEspera es el guardián de perímetro de §7.3 del diseño
// docs/design/colas-virtuales.md. Firma fija por el diseño: recibe la
// RutaProtegida que protege — nunca la deduce del path — para que la
// relación ruta↔catálogo sea explícita y grepeable. Se monta por operación,
// igual que MiddlewareAutenticacion de Acceso y
// middlewareAutorizacionTenencia (§12 del diseño: montaje real en
// Acceso/Identidad/Tenencia, fuera del alcance de esta extensión).
//
// Lógica, en el orden exacto de §7.3:
//
//  1. SalaVigentePara, sin E/S: si no hay sala vigente, next(ctx) y listo
//     — el costo de esta feature cuando ninguna sala está abierta es una
//     lectura de un puntero atómico.
//  2. Leer la cabecera X-Ticket-Cola; si falta, 503 (ticket_requerido).
//  3. Reclamar(clave, ticket): "admitido" deja pasar; cualquier otro
//     desenlace corta con 503 (§7.4).
//  4. Un error que no es ninguno de los desenlaces conocidos es una falla
//     de infraestructura (Redis caído): se aplica el ModoDegradado de la
//     VistaSalaVigente, decidido sin volver a tocar Redis (§8 del diseño).
func MiddlewareSalaDeEspera(api huma.API, portero puertos.PorteroDeSala, ruta dominio.RutaProtegida) func(huma.Context, func(huma.Context)) {
	rutaTexto := ruta.String()
	return func(ctx huma.Context, next func(huma.Context)) {
		vista, hay := portero.SalaVigentePara(ctx.Context(), puertos.ConsultaSalaVigente{
			Ruta:           rutaTexto,
			IDOrganizacion: idOrganizacionAutorizadaDesdeContexto(ctx.Context()),
		})
		if !hay {
			next(ctx)
			return
		}

		ticket := strings.TrimSpace(ctx.Header(cabeceraTicketCola))
		if ticket == "" {
			escribirBloqueoSala(api, ctx, vista.Alias, "ticket_requerido", 0, 0, retryTicketRequeridoSegundos)
			return
		}

		resultado, err := portero.Reclamar(ctx.Context(), puertos.ComandoReclamarTurno{
			Clave:       vista.Clave,
			TicketPlano: ticket,
		})
		if err == nil {
			// "admitido": la única operación que puede dejar pasar la
			// petición real (INV-COLA-06).
			next(ctx)
			return
		}

		if desenlace, ok := desenlaceDesdeErrorReclamo(err); ok {
			retrySegundos := retrySegundosDesdeDesenlace(desenlace, resultado)
			escribirBloqueoSala(api, ctx, vista.Alias, desenlace, resultado.Posicion, segundosCeil(resultado.EsperaEstimada), retrySegundos)
			return
		}

		// Error de infraestructura (Redis caído): se aplica el
		// ModoDegradado de la VistaSalaVigente, decidido SIN volver a
		// tocar Redis (viene de la instantánea en memoria del
		// reconciliador, §3.7/§8 del diseño).
		if vista.ModoDegradado == dominio.ModoDegradadoPermitir.String() {
			slog.ErrorContext(ctx.Context(), "confianza: el estado de la cola no respondió; se deja pasar por modoDegradado=permitir (fail-open, §8 del diseño)",
				"alias_sala", vista.Alias, "clave", vista.Clave, "error", err)
			next(ctx)
			return
		}

		slog.ErrorContext(ctx.Context(), "confianza: el estado de la cola no respondió; se rechaza por modoDegradado=rechazar (fail-closed, §8 del diseño)",
			"alias_sala", vista.Alias, "clave", vista.Clave, "error", err)
		escribirBloqueoSala(api, ctx, vista.Alias, "sala_no_disponible", 0, 0, retrySalaNoDisponibleSegundos)
	}
}

// desenlaceDesdeErrorReclamo traduce un error devuelto por
// PorteroDeSala.Reclamar a su desenlace textual (§1.8 y §7.4 del diseño),
// cuando err es uno de los cinco tipos de dominio que
// aplicacion.PorteroDeSalaCasoDeUso.Reclamar puede producir. Cualquier otro
// error (p. ej. el error de infraestructura sin envolver que devuelve
// EstadoDeCola cuando Redis no responde) se trata como falla y el segundo
// valor de retorno es false.
func desenlaceDesdeErrorReclamo(err error) (string, bool) {
	var (
		errTurnoNoAlcanzado  *dominio.ErrTurnoNoAlcanzado
		errTurnoCaducado     *dominio.ErrTurnoCaducado
		errTicketDesconocido *dominio.ErrTicketDesconocido
		errTicketConsumido   *dominio.ErrTicketConsumido
		errSalaNoAbierta     *dominio.ErrSalaNoAbierta
		errColaLlena         *dominio.ErrColaLlena
	)
	switch {
	case errors.As(err, &errTurnoNoAlcanzado):
		return dominio.DesenlaceEsperando.String(), true
	case errors.As(err, &errTurnoCaducado):
		return dominio.DesenlaceTurnoCaducado.String(), true
	case errors.As(err, &errTicketDesconocido):
		return dominio.DesenlaceTicketDesconocido.String(), true
	case errors.As(err, &errTicketConsumido):
		return dominio.DesenlaceTicketConsumido.String(), true
	case errors.As(err, &errSalaNoAbierta):
		return dominio.DesenlaceSalaCerrada.String(), true
	case errors.As(err, &errColaLlena):
		return dominio.DesenlaceColaLlena.String(), true
	default:
		return "", false
	}
}

// retrySegundosDesdeDesenlace calcula el Retry-After a partir de los datos
// que ya trae puertos.ResultadoTurno (§7.4 del ejemplo del diseño:
// "Retry-After: 257" coincide exactamente con
// "espera_estimada_segundos": 257 para el desenlace "esperando"). Para el
// resto de los desenlaces conocidos, el diseño no da un número: se usa
// ResultadoTurno.ReconsultarEn (el mismo intervalo de sondeo dictado por el
// servidor que ya usan los endpoints públicos), con un piso corto cuando no
// aporta nada útil.
func retrySegundosDesdeDesenlace(desenlace string, resultado puertos.ResultadoTurno) int {
	if desenlace == dominio.DesenlaceEsperando.String() {
		segundos := int(segundosCeil(resultado.EsperaEstimada))
		if segundos < 1 {
			segundos = 1
		}
		return segundos
	}
	segundos := int(segundosCeil(resultado.ReconsultarEn))
	if segundos < 1 {
		segundos = retryDesenlaceGenericoSegundos
	}
	return segundos
}

// segundosCeil redondea una duración hacia arriba en segundos enteros,
// nunca negativo: un cliente nunca debe reintentar antes de lo indicado.
func segundosCeil(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return int64(math.Ceil(d.Seconds()))
}

// escribirBloqueoSala escribe el cuerpo de bloqueo de §7.4 del diseño con
// content negotiation (api.Negotiate/api.Marshal, la misma infraestructura
// que usa Huma internamente para cualquier otra respuesta) en vez de
// huma.WriteErr, porque el cuerpo necesita campos que huma.ErrorModel no
// tiene lugar para transportar.
func escribirBloqueoSala(api huma.API, ctx huma.Context, aliasSala, desenlace string, posicion, esperaEstimadaSegundos int64, retrySegundos int) {
	if retrySegundos < 1 {
		retrySegundos = 1
	}
	cuerpo := cuerpoBloqueoSala{
		Type:                   tipoErrorSalaDeEspera,
		Title:                  tituloSalaDeEspera,
		Status:                 http.StatusServiceUnavailable,
		Detail:                 detalleSalaDeEspera,
		Desenlace:              desenlace,
		AliasSala:              aliasSala,
		Posicion:               posicion,
		EsperaEstimadaSegundos: esperaEstimadaSegundos,
		Ingreso:                "/confianza/salas-espera/" + aliasSala + "/tickets",
		EstadoTurno:            "/confianza/salas-espera/" + aliasSala + "/turno",
	}

	ct, err := api.Negotiate(ctx.Header("Accept"))
	if err != nil || ct == "application/json" {
		ct = "application/problem+json"
	}
	ctx.SetHeader("Content-Type", ct)
	ctx.SetHeader("Retry-After", strconv.Itoa(retrySegundos))
	ctx.SetStatus(http.StatusServiceUnavailable)
	if err := api.Marshal(ctx.BodyWriter(), ct, cuerpo); err != nil {
		slog.ErrorContext(ctx.Context(), "confianza: no se pudo serializar el cuerpo de bloqueo de sala de espera", "error", err)
	}
}
