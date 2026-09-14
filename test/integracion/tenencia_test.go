// Este archivo contiene los tests de integración del bounded context
// Tenencia: ejercitan la pila real (HTTP -> aplicacion -> Postgres real,
// con RLS activo y los triggers de auditoría corriendo en la base de
// datos), sin mocks. Comparte el paquete integracion con los tests de
// Identidad y Acceso (mismo criterio que entorno_test.go) y reutiliza sus
// helpers de bajo nivel (poolAplicacion, poolDueno, correoUnico,
// activarUsuario, usuarioActivoDePrueba, iniciarSesionDePrueba,
// limpiarSesionesDeUsuario, borrarUsuario, respuestaHTTP, peticionJSON,
// errorHumaRespuesta, assertCadenaAuditoriaIntegra — todos definidos en
// entorno_test.go/acceso_test.go).
//
// El test más importante de este archivo es
// TestTenencia_RLS_AislamientoEntreOrganizaciones_BloqueaCrossTenant: es la
// formalización automatizada del bug real de seguridad que se encontró y
// corrigió a mano en internal/plataforma/bd/alcance_tenencia.go
// (ReforzarAlcanceTenencia reemitía el alcance de RLS con los parámetros de
// la propia consulta, comparando la política contra sí misma). Ver el
// comentario de cabecera de ese archivo antes de tocar este test.
package integracion

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	accesoauditoria "github.com/r-david1/moterus/internal/acceso/adaptadores/auditoria"
	accesoconfianza "github.com/r-david1/moterus/internal/acceso/adaptadores/confianza"
	accesocripto "github.com/r-david1/moterus/internal/acceso/adaptadores/cripto"
	accesoeventos "github.com/r-david1/moterus/internal/acceso/adaptadores/eventos"
	accesohttp "github.com/r-david1/moterus/internal/acceso/adaptadores/http"
	accesoidentidad "github.com/r-david1/moterus/internal/acceso/adaptadores/identidad"
	accesojwt "github.com/r-david1/moterus/internal/acceso/adaptadores/jwt"
	accesopostgres "github.com/r-david1/moterus/internal/acceso/adaptadores/postgres"
	accesoredis "github.com/r-david1/moterus/internal/acceso/adaptadores/redis"
	accesoaplicacion "github.com/r-david1/moterus/internal/acceso/aplicacion"
	accesodominio "github.com/r-david1/moterus/internal/acceso/dominio"
	accesopuertos "github.com/r-david1/moterus/internal/acceso/puertos"
	accesomocks "github.com/r-david1/moterus/internal/acceso/puertos/mocks"

	"github.com/r-david1/moterus/internal/identidad/adaptadores/auditoria"
	identidadconfianza "github.com/r-david1/moterus/internal/identidad/adaptadores/confianza"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/cripto"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/eventos"
	identidadhttp "github.com/r-david1/moterus/internal/identidad/adaptadores/http"
	"github.com/r-david1/moterus/internal/identidad/adaptadores/notificaciones"
	identidadpostgres "github.com/r-david1/moterus/internal/identidad/adaptadores/postgres"
	"github.com/r-david1/moterus/internal/identidad/aplicacion"

	"github.com/r-david1/moterus/internal/plataforma/bd"
	"github.com/r-david1/moterus/internal/plataforma/cache"
	"github.com/r-david1/moterus/internal/plataforma/reloj"

	tenenciaauditoria "github.com/r-david1/moterus/internal/tenencia/adaptadores/auditoria"
	tenenciaconfianza "github.com/r-david1/moterus/internal/tenencia/adaptadores/confianza"
	tenenciacripto "github.com/r-david1/moterus/internal/tenencia/adaptadores/cripto"
	tenenciaeventos "github.com/r-david1/moterus/internal/tenencia/adaptadores/eventos"
	tenenciahttp "github.com/r-david1/moterus/internal/tenencia/adaptadores/http"
	tenenciaidentidad "github.com/r-david1/moterus/internal/tenencia/adaptadores/identidad"
	tenenciapostgres "github.com/r-david1/moterus/internal/tenencia/adaptadores/postgres"
	tenenciaaplicacion "github.com/r-david1/moterus/internal/tenencia/aplicacion"
	tenenciadominio "github.com/r-david1/moterus/internal/tenencia/dominio"
	tenenciapuertos "github.com/r-david1/moterus/internal/tenencia/puertos"
)

// --- ensamblaje del servidor bajo prueba (Identidad + Acceso + Tenencia) ---

// capturaInvitaciones implementa tenenciapuertos.NotificadorInvitaciones en
// memoria, en vez del NotificadorInvitacionesLog real (log-only, ADR de
// Tenencia §3.5): estos tests necesitan el token en claro para poder
// ejercer POST /tenencia/invitaciones/aceptaciones, exactamente el mismo
// motivo por el que verificacion_correo_test.go de Identidad inyecta su
// propio captor en vez del NotificadorCorreoLog real.
type capturaInvitaciones struct {
	mu           sync.Mutex
	ultimoToken  string
	ultimoCorreo string
	ultimaExpira time.Time
	invocaciones int
}

func (c *capturaInvitaciones) EnviarInvitacion(_ context.Context, destinatario tenenciadominio.CorreoDestinatario, _ string, _ tenenciadominio.Rol, tokenPlano string, expiraEn time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ultimoToken = tokenPlano
	c.ultimoCorreo = destinatario.Normalizado()
	c.ultimaExpira = expiraEn
	c.invocaciones++
	return nil
}

func (c *capturaInvitaciones) token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ultimoToken
}

var _ tenenciapuertos.NotificadorInvitaciones = (*capturaInvitaciones)(nil)

// nuevoServidorTenencia reproduce el mismo cableado que
// cmd/api/main.go#montarIdentidadYAcceso + montarTenencia: Identidad y
// Acceso exactamente como nuevoServidorAcceso (acceso_test.go), más los
// tres repositorios Postgres de Tenencia con RLS real (contra
// dsnAplicacion, es decir rol_login_identidad — nunca el rol dueño,
// mismo criterio que el resto de este paquete), los ACL de cruce y las
// rutas HTTP con sus dos middlewares. Devuelve también la
// *capturaInvitaciones inyectada, para que los tests de invitaciones
// puedan leer el token en claro que nunca sale por HTTP (INV-TEN-23).
func nuevoServidorTenencia(t *testing.T, pool *pgxpool.Pool) (*fiber.App, *capturaInvitaciones) {
	t.Helper()

	relojReal := reloj.NuevoReal()
	loggerSilencioso := slog.New(slog.NewTextHandler(io.Discard, nil))

	// --- Identidad ----------------------------------------------------------
	repositorioUsuarios := identidadpostgres.NuevoRepositorioUsuarios(pool)
	repositorioTokensVerificacion := identidadpostgres.NuevoRepositorioTokensVerificacion(pool)
	unidadDeTrabajoIdentidad := identidadpostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDsIdentidad := identidadpostgres.NuevoGeneradorIDs()

	hasher := cripto.NuevoHasherArgon2id()
	verificadorFiltradas := cripto.NuevoVerificadorHIBP(cripto.ConBaseURLHIBP(servidorHIBPPruebas.URL + "/range/"))
	generadorTokensIdentidad := cripto.NuevoGeneradorTokens()

	registroAuditoriaIdentidad := auditoria.NuevoRegistroAuditoria(pool)
	publicadorEventosIdentidad := eventos.NuevoPublicadorLog(loggerSilencioso)
	evaluadorConfianzaIdentidad := identidadconfianza.NuevoEvaluadorConfianzaNoOp(loggerSilencioso)
	notificadorCorreo := notificaciones.NuevoNotificadorCorreoLog(loggerSilencioso)

	registrador := aplicacion.NuevoRegistrarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, verificadorFiltradas, evaluadorConfianzaIdentidad,
		registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, generadorIDsIdentidad, unidadDeTrabajoIdentidad,
		generadorTokensIdentidad, repositorioTokensVerificacion, notificadorCorreo,
	)
	autenticadorIdentidad := aplicacion.NuevoAutenticarUsuarioCasoDeUso(
		repositorioUsuarios, hasher, evaluadorConfianzaIdentidad,
		registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, unidadDeTrabajoIdentidad,
	)
	consultorIdentidad := aplicacion.NuevoObtenerUsuarioCasoDeUso(repositorioUsuarios, registroAuditoriaIdentidad, relojReal)
	verificadorCorreo := aplicacion.NuevoVerificarCorreoCasoDeUso(
		repositorioUsuarios, repositorioTokensVerificacion, registroAuditoriaIdentidad, publicadorEventosIdentidad, relojReal, unidadDeTrabajoIdentidad,
	)
	reenviadorVerificacion := aplicacion.NuevoReenviarVerificacionCasoDeUso(
		repositorioUsuarios, generadorTokensIdentidad, repositorioTokensVerificacion, notificadorCorreo, relojReal,
	)
	manejadorIdentidad := identidadhttp.NuevoManejadorIdentidad(
		registrador, autenticadorIdentidad, consultorIdentidad, verificadorCorreo, reenviadorVerificacion, evaluadorConfianzaIdentidad, nil, nil,
	)

	// --- Acceso ---------------------------------------------------------------
	politicaSesion := accesodominio.PoliticaSesionPorDefecto()

	sesiones := accesopostgres.NuevoRepositorioSesiones(pool)
	uowAcceso := accesopostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDsAcceso := accesopostgres.NuevoGeneradorIDs()
	generadorRefrescos := accesocripto.NuevoGeneradorTokensRefresco()
	registroAuditoriaAcceso := accesoauditoria.NuevoRegistroAuditoria(pool)
	publicadorEventosAcceso := accesoeventos.NuevoPublicadorLog(loggerSilencioso)

	llaveEfimera, err := accesojwt.GenerarLlaveEfimera()
	if err != nil {
		t.Fatalf("GenerarLlaveEfimera(): %v", err)
	}
	llavero, err := accesojwt.NuevoLlavero(llaveEfimera, nil)
	if err != nil {
		t.Fatalf("NuevoLlavero(): %v", err)
	}
	firmador := accesojwt.NuevoFirmador(llavero, "https://acceso.test.moterus.local", "moterus-test", politicaSesion.ToleranciaReloj())

	var listaRevocacion accesopuertos.ListaRevocacion
	if redisDisponible(t) {
		clienteRedis, err := cache.NuevoClienteRedis(mustRedisURL(t))
		if err != nil {
			t.Fatalf("NuevoClienteRedis(): %v", err)
		}
		t.Cleanup(func() { _ = clienteRedis.Close() })
		listaRevocacion = accesoredis.NuevaListaRevocacion(clienteRedis)
	} else {
		listaRevocacion = accesoredis.NuevaListaRevocacionNoOp()
	}

	evaluadorConfianzaAcceso := accesoconfianza.NuevoEvaluadorConfianzaNoOp(loggerSilencioso)

	autenticadorACL := accesoidentidad.NuevoAutenticadorIdentidad(autenticadorIdentidad)
	consultorEstadoSujeto := accesoidentidad.NuevoConsultorEstadoSujeto(consultorIdentidad)

	// emisorStepUp: ver el mismo comentario en acceso_test.go — adaptador
	// real pendiente (ADR 0038), fuera del alcance de este cambio; ningún
	// test de este archivo ejercita step-up/MFA.
	iniciador := accesoaplicacion.NuevoIniciarSesionCasoDeUso(
		autenticadorACL, sesiones, generadorRefrescos, firmador, listaRevocacion,
		registroAuditoriaAcceso, publicadorEventosAcceso, relojReal, generadorIDsAcceso, uowAcceso, politicaSesion,
		"https://acceso.test.moterus.local", "moterus-test",
		&accesomocks.EmisorTokenStepUp{},
	)
	renovador := accesoaplicacion.NuevoRenovarSesionCasoDeUso(
		evaluadorConfianzaAcceso, sesiones, generadorRefrescos, firmador, consultorEstadoSujeto, listaRevocacion,
		registroAuditoriaAcceso, relojReal, generadorIDsAcceso, uowAcceso, politicaSesion,
		"https://acceso.test.moterus.local", "moterus-test",
	)
	validador := accesoaplicacion.NuevoValidarAccesoCasoDeUso(firmador, listaRevocacion, sesiones, registroAuditoriaAcceso, relojReal)
	cerrador := accesoaplicacion.NuevoCerrarSesionCasoDeUso(sesiones, evaluadorConfianzaAcceso, listaRevocacion, registroAuditoriaAcceso, relojReal, uowAcceso, politicaSesion)
	consultorSesiones := accesoaplicacion.NuevoListarSesionesCasoDeUso(sesiones)

	manejadorAcceso := accesohttp.NuevoManejadorAcceso(iniciador, renovador, cerrador, consultorSesiones, firmador, nil)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	accesohttp.RegistrarRutas(app, manejadorAcceso, validador, nil)
	identidadhttp.RegistrarRutas(app, manejadorIdentidad, validador, nil)

	// --- Tenencia ---------------------------------------------------------------
	organizaciones := tenenciapostgres.NuevoRepositorioOrganizaciones(pool)
	membresias := tenenciapostgres.NuevoRepositorioMembresias(pool)
	invitacionesRepo := tenenciapostgres.NuevoRepositorioInvitaciones(pool)
	uowTenencia := tenenciapostgres.NuevaUnidadDeTrabajo(pool)
	generadorIDsTenencia := tenenciapostgres.NuevoGeneradorIDs()
	alcance := tenenciapostgres.NuevaAlcanceTenencia()
	generadorTokensTenencia := tenenciacripto.NuevoGeneradorTokens()

	registroAuditoriaTenencia := tenenciaauditoria.NuevoRegistroAuditoria(pool)
	publicadorEventosTenencia := tenenciaeventos.NuevoPublicadorLog(loggerSilencioso)
	captura := &capturaInvitaciones{}
	sujetos := tenenciaidentidad.NuevoVerificadorSujetos(consultorIdentidad)
	evaluadorConfianzaTenencia := tenenciaconfianza.NuevoEvaluadorConfianzaNoOp(loggerSilencioso)

	politicaOrganizacion := tenenciadominio.PoliticaOrganizacionPorDefecto()

	autorizador := tenenciaaplicacion.NuevoAutorizarCasoDeUso(organizaciones, membresias, registroAuditoriaTenencia, relojReal)
	gestorOrganizaciones := tenenciaaplicacion.NuevoOrganizacionesCasoDeUso(
		organizaciones, membresias, autorizador, sujetos, evaluadorConfianzaTenencia,
		registroAuditoriaTenencia, publicadorEventosTenencia, relojReal, generadorIDsTenencia, uowTenencia, politicaOrganizacion,
	)
	gestorMembresias := tenenciaaplicacion.NuevoMembresiasCasoDeUso(
		organizaciones, membresias, autorizador, sujetos,
		registroAuditoriaTenencia, publicadorEventosTenencia, relojReal, generadorIDsTenencia, uowTenencia, politicaOrganizacion,
	)
	gestorInvitaciones := tenenciaaplicacion.NuevoInvitacionesCasoDeUso(
		organizaciones, membresias, invitacionesRepo, autorizador, sujetos, evaluadorConfianzaTenencia, captura,
		registroAuditoriaTenencia, publicadorEventosTenencia, relojReal, generadorIDsTenencia, generadorTokensTenencia, uowTenencia, politicaOrganizacion,
	)
	consultas := tenenciaaplicacion.NuevoConsultasCasoDeUso(organizaciones, membresias, autorizador)

	manejadorTenencia := tenenciahttp.NuevoManejadorTenencia(gestorOrganizaciones, consultas, gestorMembresias, consultas, gestorInvitaciones)
	tenenciahttp.RegistrarRutas(app, manejadorTenencia, validador, autorizador, alcance, nil)

	return app, captura
}

// --- fixtures propias de Tenencia -------------------------------------------

// aliasUnico genera un alias de organización válido (minúsculas, dígitos,
// guiones simples, 3-48 caracteres) y único por ejecución de test, mismo
// criterio que correoUnico.
func aliasUnico(t *testing.T, prefijo string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d-%d", prefijo, time.Now().UnixNano(), rand.Intn(1_000_000)) //nolint:gosec // sufijo de unicidad de datos de prueba, no un secreto ni una decisión de seguridad.
}

// crearOrganizacionDePrueba crea una organización vía HTTP con el token de
// acceso dado (que se vuelve automáticamente su propietario, INV-TEN-03) y
// devuelve su ID.
func crearOrganizacionDePrueba(t *testing.T, app *fiber.App, tokenAcceso, nombre, alias string) string {
	t.Helper()
	var resultado struct {
		ID string `json:"id"`
	}
	req := peticionJSON(t, http.MethodPost, "/tenencia/organizaciones", map[string]any{
		"nombre": nombre,
		"alias":  alias,
	})
	req.Header.Set("Authorization", "Bearer "+tokenAcceso)
	status := respuestaHTTP(t, app, req, &resultado)
	if status != http.StatusCreated {
		t.Fatalf("crear organización de prueba (alias=%q): status=%d", alias, status)
	}
	if resultado.ID == "" {
		t.Fatalf("crear organización de prueba: la respuesta no trajo id")
	}
	return resultado.ID
}

// agregarMiembroDePrueba agrega idUsuario a idOrganizacion con el rol dado,
// ejecutado por tokenEjecutor (alta directa, §3.3 del diseño).
func agregarMiembroDePrueba(t *testing.T, app *fiber.App, tokenEjecutor, idOrganizacion, idUsuario, rol string) {
	t.Helper()
	req := peticionJSON(t, http.MethodPost, "/tenencia/organizaciones/"+idOrganizacion+"/miembros", map[string]any{
		"usuario_id": idUsuario,
		"rol":        rol,
	})
	req.Header.Set("Authorization", "Bearer "+tokenEjecutor)
	status := respuestaHTTP(t, app, req, nil)
	if status != http.StatusCreated {
		t.Fatalf("agregar miembro de prueba (org=%s usuario=%s rol=%s): status=%d", idOrganizacion, idUsuario, rol, status)
	}
}

// borrarOrganizacion limpia (con el rol dueño, que bypassa RLS y ACL por
// ser superusuario del docker-compose de desarrollo — ver comentario de
// dsnDueno en entorno_test.go) todo lo que un test haya creado bajo una
// organización: invitaciones, membresías y la organización, EN UNA SOLA
// TRANSACCIÓN explícita. Esto es necesario (no solo prolijo): el
// CONSTRAINT TRIGGER diferido de INV-TEN-06
// (membresias_al_menos_un_propietario, migración 000009) se evalúa recién
// al COMMIT, así que si el DELETE de membresias y el de organizaciones
// fueran dos Exec sueltos (cada uno su propia transacción implícita), el
// primero dispararía el trigger con la organización todavía viva y SIN
// propietario -> ERROR "quedaria sin propietario activo". Al hacer los tres
// DELETE en la misma transacción, para cuando el trigger se evalúa (COMMIT)
// la fila de organizaciones ya no existe y el trigger es un no-op (ver
// tenencia_verificar_propietario: `SELECT estado INTO v_estado_org ...`
// devuelve NULL, `IS DISTINCT FROM 'activa'` es verdadero, `RETURN NULL`).
func borrarOrganizacion(t *testing.T, poolDueno *pgxpool.Pool, idOrganizacion string) {
	t.Helper()
	if idOrganizacion == "" {
		return
	}
	ctx := context.Background()
	tx, err := poolDueno.Begin(ctx)
	if err != nil {
		t.Logf("no se pudo abrir la transacción de limpieza de la organización %s: %v", idOrganizacion, err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM invitaciones WHERE organizacion_id = $1`, idOrganizacion); err != nil {
		t.Logf("no se pudieron limpiar las invitaciones de prueba de la organización %s: %v", idOrganizacion, err)
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM membresias WHERE organizacion_id = $1`, idOrganizacion); err != nil {
		t.Logf("no se pudieron limpiar las membresías de prueba de la organización %s: %v", idOrganizacion, err)
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM organizaciones WHERE id = $1`, idOrganizacion); err != nil {
		t.Logf("no se pudo limpiar la organización de prueba %s: %v", idOrganizacion, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		t.Logf("no se pudo confirmar la limpieza de la organización de prueba %s: %v", idOrganizacion, err)
	}
}

// --- 1. Flujo feliz de punta a punta por HTTP --------------------------------

// TestTenencia_FlujoCompletoHTTP_CrearOrganizacionYMiembros cubre: registrar
// y activar dos usuarios, loguear con Acceso, crear una organización con el
// primero, verificar que el segundo (sin membresía) recibe 404 al intentar
// verla o listar sus miembros (INV-TEN-17), agregarlo como miembro, y
// verificar que ahora sí puede.
func TestTenencia_FlujoCompletoHTTP_CrearOrganizacionYMiembros(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app, _ := nuevoServidorTenencia(t, pool)

	idA, correoA := usuarioActivoDePrueba(t, app, dueno, "flujo-a")
	t.Cleanup(func() { borrarUsuario(t, dueno, idA) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idA) })
	tokenA := iniciarSesionDePrueba(t, app, correoA).TokenAcceso

	idB, correoB := usuarioActivoDePrueba(t, app, dueno, "flujo-b")
	t.Cleanup(func() { borrarUsuario(t, dueno, idB) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idB) })
	tokenB := iniciarSesionDePrueba(t, app, correoB).TokenAcceso

	idOrg := crearOrganizacionDePrueba(t, app, tokenA, "Organización de Flujo Completo", aliasUnico(t, "flujo"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrg) })

	// B no es miembro todavía: 404 en ambos endpoints, nunca 403 (INV-TEN-17).
	reqObtener := peticionJSON(t, http.MethodGet, "/tenencia/organizaciones/"+idOrg, nil)
	reqObtener.Header.Set("Authorization", "Bearer "+tokenB)
	var errResp errorHumaRespuesta
	if status := respuestaHTTP(t, app, reqObtener, &errResp); status != http.StatusNotFound {
		t.Fatalf("GET organización sin membresía: status=%d, se esperaba 404", status)
	}

	reqMiembros := peticionJSON(t, http.MethodGet, "/tenencia/organizaciones/"+idOrg+"/miembros", nil)
	reqMiembros.Header.Set("Authorization", "Bearer "+tokenB)
	if status := respuestaHTTP(t, app, reqMiembros, &errResp); status != http.StatusNotFound {
		t.Fatalf("GET miembros sin membresía: status=%d, se esperaba 404", status)
	}

	// A (propietario) agrega a B como miembro.
	agregarMiembroDePrueba(t, app, tokenA, idOrg, idB, "miembro")

	// Ahora B sí puede verla.
	reqObtener2 := peticionJSON(t, http.MethodGet, "/tenencia/organizaciones/"+idOrg, nil)
	reqObtener2.Header.Set("Authorization", "Bearer "+tokenB)
	var vistaOrg struct {
		ID              string `json:"id"`
		MiembrosActivos int    `json:"miembros_activos"`
	}
	if status := respuestaHTTP(t, app, reqObtener2, &vistaOrg); status != http.StatusOK {
		t.Fatalf("GET organización siendo miembro: status=%d, se esperaba 200", status)
	}
	if vistaOrg.ID != idOrg {
		t.Fatalf("id de la organización = %q, se esperaba %q", vistaOrg.ID, idOrg)
	}
	if vistaOrg.MiembrosActivos != 2 {
		t.Fatalf("miembros_activos = %d, se esperaban 2", vistaOrg.MiembrosActivos)
	}

	reqMiembros2 := peticionJSON(t, http.MethodGet, "/tenencia/organizaciones/"+idOrg+"/miembros", nil)
	reqMiembros2.Header.Set("Authorization", "Bearer "+tokenB)
	var miembros []map[string]any
	if status := respuestaHTTP(t, app, reqMiembros2, &miembros); status != http.StatusOK {
		t.Fatalf("GET miembros siendo miembro: status=%d, se esperaba 200", status)
	}
	if len(miembros) != 2 {
		t.Fatalf("se esperaban 2 miembros, hubo %d", len(miembros))
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// --- 2. RLS: aislamiento cross-tenant (el test más importante de esta fase) -

// TestTenencia_RLS_AislamientoEntreOrganizaciones_BloqueaCrossTenant es la
// formalización automatizada del bug real de seguridad que se encontró y
// corrigió a mano en bd.ReforzarAlcanceTenencia: antes del fix, la función
// reemitía el SET LOCAL de RLS con los parámetros de la propia consulta,
// haciendo que la política se comparara consigo misma
// ("organizacion_id = organizacion_id", siempre verdadero) y nunca pudiera
// bloquear una fuga cross-tenant.
//
// No alcanza con probar esto por HTTP (el middleware de autorización de
// aplicación ya bloquearía correctamente incluso sin RLS). Este test
// ejercita el adaptador Postgres de Tenencia DIRECTAMENTE: publica en el
// ctx un AlcanceTenencia ya "autorizado" para la organización A (como lo
// haría el middleware HTTP tras verificar un permiso sobre A) y luego pide
// al repositorio, DENTRO de esa misma transacción, datos de la organización
// B — pasando el ID de B como parámetro de la propia consulta, exactamente
// el escenario que el bug real habría permitido. Si RLS es una red de
// seguridad real (post-fix), la consulta debe devolver CERO filas: el
// alcance ya fijado (A) nunca se reemplaza por los parámetros de una
// consulta posterior.
func TestTenencia_RLS_AislamientoEntreOrganizaciones_BloqueaCrossTenant(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app, _ := nuevoServidorTenencia(t, pool)

	idUsuarioA, correoA := usuarioActivoDePrueba(t, app, dueno, "rls-a")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuarioA) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuarioA) })
	tokenA := iniciarSesionDePrueba(t, app, correoA).TokenAcceso
	idOrgA := crearOrganizacionDePrueba(t, app, tokenA, "Organización A (RLS)", aliasUnico(t, "rls-a"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrgA) })

	idUsuarioB, correoB := usuarioActivoDePrueba(t, app, dueno, "rls-b")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuarioB) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuarioB) })
	tokenB := iniciarSesionDePrueba(t, app, correoB).TokenAcceso
	idOrgB := crearOrganizacionDePrueba(t, app, tokenB, "Organización B (RLS)", aliasUnico(t, "rls-b"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrgB) })

	// Confirmación (con el rol dueño, que bypassa RLS por ser superusuario)
	// de que la organización B efectivamente tiene una membresía fundacional
	// vigente: si el test de abajo diera 0 filas por una razón trivial
	// (nunca se creó nada), esto lo descartaría.
	var totalMembresiasB int
	if err := dueno.QueryRow(context.Background(),
		`SELECT count(*) FROM membresias WHERE organizacion_id = $1 AND estado = 'activa'`, idOrgB,
	).Scan(&totalMembresiasB); err != nil {
		t.Fatalf("verificando membresías de B con el rol dueño: %v", err)
	}
	if totalMembresiasB == 0 {
		t.Fatalf("la organización B no tiene ninguna membresía activa; el test no probaría nada")
	}

	membresias := tenenciapostgres.NuevoRepositorioMembresias(pool)
	uow := tenenciapostgres.NuevaUnidadDeTrabajo(pool)

	idOrgADominio, err := tenenciadominio.IDOrganizacionDesde(idOrgA)
	if err != nil {
		t.Fatalf("IDOrganizacionDesde(idOrgA): %v", err)
	}
	idOrgBDominio, err := tenenciadominio.IDOrganizacionDesde(idOrgB)
	if err != nil {
		t.Fatalf("IDOrganizacionDesde(idOrgB): %v", err)
	}
	idUsuarioBDominio, err := tenenciadominio.IDUsuarioDesde(idUsuarioB)
	if err != nil {
		t.Fatalf("IDUsuarioDesde(idUsuarioB): %v", err)
	}

	// Alcance YA autorizado para la organización A (como lo dejaría el
	// middleware de autorización tras un Autorizar exitoso sobre A).
	ctxConAlcanceA := bd.ConAlcanceTenencia(context.Background(), idUsuarioA, idOrgA)

	// --- El caso que el bug real habría permitido -----------------------
	var membresiasDeB []*tenenciadominio.Membresia
	if err := uow.Ejecutar(ctxConAlcanceA, func(ctx context.Context) error {
		var errInterno error
		// El repositorio recibe idOrgB como parámetro EXPLÍCITO de esta
		// consulta puntual, exactamente como lo haría un caso de uso con un
		// bug futuro que operara sobre la organización equivocada.
		membresiasDeB, errInterno = membresias.ListarDeOrganizacion(ctx, idOrgBDominio)
		return errInterno
	}); err != nil {
		t.Fatalf("ListarDeOrganizacion(orgB) bajo alcance(orgA): %v", err)
	}
	if len(membresiasDeB) != 0 {
		t.Fatalf("RLS NO bloqueó el acceso cross-tenant: con el alcance fijado en la organización A, "+
			"ListarDeOrganizacion(orgB) devolvió %d fila(s) — esto es exactamente la regresión de seguridad "+
			"que bd.ReforzarAlcanceTenencia corrige (ver su comentario de cabecera)", len(membresiasDeB))
	}

	// Misma prueba con BuscarVigente: pedir la membresía fundacional de B en
	// su propia organización B, bajo el alcance de A, debe ser invisible.
	var membresiaVigente *tenenciadominio.Membresia
	if err := uow.Ejecutar(ctxConAlcanceA, func(ctx context.Context) error {
		var errInterno error
		membresiaVigente, errInterno = membresias.BuscarVigente(ctx, idUsuarioBDominio, idOrgBDominio)
		return errInterno
	}); err != nil {
		t.Fatalf("BuscarVigente(usuarioB, orgB) bajo alcance(orgA): %v", err)
	}
	if membresiaVigente != nil {
		t.Fatalf("RLS NO bloqueó el acceso cross-tenant: BuscarVigente(usuarioB, orgB) bajo alcance(orgA) "+
			"devolvió una membresía (id=%s) que pertenece a otra organización", membresiaVigente.ID().String())
	}

	// Control positivo: el MISMO mecanismo, con el alcance correcto (A),
	// debe seguir devolviendo los datos reales de A — para descartar que la
	// consulta "siempre da vacío" por algún motivo ajeno a RLS.
	var membresiasDeA []*tenenciadominio.Membresia
	if err := uow.Ejecutar(ctxConAlcanceA, func(ctx context.Context) error {
		var errInterno error
		membresiasDeA, errInterno = membresias.ListarDeOrganizacion(ctx, idOrgADominio)
		return errInterno
	}); err != nil {
		t.Fatalf("ListarDeOrganizacion(orgA) bajo alcance(orgA): %v", err)
	}
	if len(membresiasDeA) == 0 {
		t.Fatalf("control positivo falló: ListarDeOrganizacion(orgA) bajo su propio alcance no devolvió ninguna fila " +
			"(¿RLS está fallando ABIERTO/CERRADO para todo, no solo para el cruce?)")
	}
}

// --- 3. FORCE ROW LEVEL SECURITY: falla cerrado sin alcance -----------------

// TestTenencia_RLS_SinAlcanceFijado_RolAplicacionNoVeNada formaliza
// INV-TEN-30: rol_login_identidad (el rol de runtime, NOBYPASSRLS —
// migración 000003 — que hereda los privilegios de rol_aplicacion, el rol
// al que apuntan las políticas de 000014_rls_tenencia) no puede ver NINGUNA
// fila de organizaciones/membresias/invitaciones sin que algo haya fijado
// primero app.usuario_actual/app.organizacion_actual con SET LOCAL. No hace
// falta un rol distinto para esto (a diferencia de lo que sugeriría "probar
// el rol dueño": el rol dueño de este docker-compose de desarrollo es
// auth_service, que es superusuario y por lo tanto SIEMPRE bypassa RLS sin
// importar FORCE — no es el caso interesante. El caso interesante, y el que
// realmente prueba INV-TEN-30, es el rol de runtime).
func TestTenencia_RLS_SinAlcanceFijado_RolAplicacionNoVeNada(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app, _ := nuevoServidorTenencia(t, pool)

	idUsuario, correo := usuarioActivoDePrueba(t, app, dueno, "rls-forzado")
	t.Cleanup(func() { borrarUsuario(t, dueno, idUsuario) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idUsuario) })
	token := iniciarSesionDePrueba(t, app, correo).TokenAcceso
	idOrg := crearOrganizacionDePrueba(t, app, token, "Organización RLS Forzado", aliasUnico(t, "rls-f"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrg) })

	// Con el rol dueño (superusuario, bypassa RLS): la fila existe de
	// verdad.
	var totalConDueno int
	if err := dueno.QueryRow(context.Background(), `SELECT count(*) FROM organizaciones WHERE id = $1`, idOrg).Scan(&totalConDueno); err != nil {
		t.Fatalf("verificando la organización con el rol dueño: %v", err)
	}
	if totalConDueno != 1 {
		t.Fatalf("la organización de prueba no existe según el rol dueño; el test no probaría nada")
	}

	// Con rol_login_identidad y SIN ningún SET LOCAL de alcance (una
	// consulta ad-hoc del pool corre en su propia transacción implícita, sin
	// AlcanceTenencia publicado en ningún ctx): FORCE ROW LEVEL SECURITY
	// hace que ni siquiera el propio dueño lógico de los datos vea la fila
	// sin alcance — falla CERRADO (INV-TEN-30), nunca ABIERTO.
	var totalSinAlcance int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM organizaciones WHERE id = $1`, idOrg).Scan(&totalSinAlcance); err != nil {
		t.Fatalf("consultando organizaciones con rol_login_identidad sin alcance: %v", err)
	}
	if totalSinAlcance != 0 {
		t.Fatalf("rol_login_identidad vio la organización SIN ningún alcance de tenencia fijado: "+
			"RLS falló ABIERTO en vez de CERRADO (INV-TEN-30), total=%d", totalSinAlcance)
	}

	var totalMembresiasSinAlcance int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM membresias WHERE organizacion_id = $1`, idOrg).Scan(&totalMembresiasSinAlcance); err != nil {
		t.Fatalf("consultando membresias con rol_login_identidad sin alcance: %v", err)
	}
	if totalMembresiasSinAlcance != 0 {
		t.Fatalf("rol_login_identidad vio membresías SIN ningún alcance de tenencia fijado (INV-TEN-30), total=%d", totalMembresiasSinAlcance)
	}
}

// --- 4. Invariante del último propietario bajo concurrencia -----------------

// resultadoPeticionConcurrente es el resultado de una petición HTTP lanzada
// desde una goroutine: nunca se llama a t.Fatal/t.Error desde la propia
// goroutine (no está permitido por el paquete testing salvo desde la
// goroutine que corre el test), así que cada goroutine solo publica su
// resultado por canal y el test principal decide.
type resultadoPeticionConcurrente struct {
	status int
	err    error
}

// TestTenencia_UltimoPropietario_ConcurrenciaExactamenteUnaGana cubre
// INV-TEN-06 bajo concurrencia real: dos transacciones intentan, al mismo
// tiempo, degradarse a sí mismas de propietario a administrador —
// exactamente una debe ganar (200) y la otra debe fallar con 409
// (ErrUltimoPropietario, o ErrConcurrenciaMembresia si el CONSTRAINT
// TRIGGER diferido de la migración 000009 llega a dispararse como red de
// seguridad). El diseño usa auto-degradación (cada ejecutor actúa sobre su
// PROPIA membresía) a propósito: así ninguna de las dos peticiones cambia
// el rol con el que la OTRA se autoriza a sí misma, y el resultado es
// determinístico sin importar el orden real de ejecución — a diferencia de
// un diseño donde A degrada a B y B degrada a A a la vez, que introduciría
// una dependencia de temporización entre la autorización de cada petición y
// el commit de la otra.
func TestTenencia_UltimoPropietario_ConcurrenciaExactamenteUnaGana(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app, _ := nuevoServidorTenencia(t, pool)

	idA, correoA := usuarioActivoDePrueba(t, app, dueno, "conc-a")
	t.Cleanup(func() { borrarUsuario(t, dueno, idA) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idA) })
	tokenA := iniciarSesionDePrueba(t, app, correoA).TokenAcceso

	idB, correoB := usuarioActivoDePrueba(t, app, dueno, "conc-b")
	t.Cleanup(func() { borrarUsuario(t, dueno, idB) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idB) })
	tokenB := iniciarSesionDePrueba(t, app, correoB).TokenAcceso

	idOrg := crearOrganizacionDePrueba(t, app, tokenA, "Organización de Concurrencia", aliasUnico(t, "conc"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrg) })

	// A agrega a B como copropietario: ahora hay exactamente 2 propietarios
	// activos.
	agregarMiembroDePrueba(t, app, tokenA, idOrg, idB, "propietario")

	lanzarAutoDegradacion := func(token, idUsuario string) resultadoPeticionConcurrente {
		req := peticionJSON(t, http.MethodPatch, "/tenencia/organizaciones/"+idOrg+"/miembros/"+idUsuario, map[string]any{
			"rol": "administrador",
		})
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req, 10_000)
		if err != nil {
			return resultadoPeticionConcurrente{err: err}
		}
		defer func() { _ = resp.Body.Close() }()
		return resultadoPeticionConcurrente{status: resp.StatusCode}
	}

	resultados := make(chan resultadoPeticionConcurrente, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		resultados <- lanzarAutoDegradacion(tokenA, idA) // A se auto-degrada
	}()
	go func() {
		defer wg.Done()
		resultados <- lanzarAutoDegradacion(tokenB, idB) // B se auto-degrada
	}()
	wg.Wait()
	close(resultados)

	var exitos, conflictos int
	var statuses []int
	for r := range resultados {
		if r.err != nil {
			t.Fatalf("error en la petición concurrente de auto-degradación: %v", r.err)
		}
		statuses = append(statuses, r.status)
		switch r.status {
		case http.StatusOK:
			exitos++
		case http.StatusConflict:
			conflictos++
		}
	}
	if exitos != 1 || conflictos != 1 {
		t.Fatalf("se esperaba exactamente 1 éxito (200) y 1 conflicto (409) en la carrera de auto-degradación "+
			"concurrente del último propietario (INV-TEN-06); statuses=%v", statuses)
	}

	// El invariante debe sostenerse en la base de datos: exactamente 1
	// propietario activo, nunca 0 ni 2.
	var propietariosActivos int
	if err := dueno.QueryRow(context.Background(),
		`SELECT count(*) FROM membresias WHERE organizacion_id = $1 AND rol = 'propietario' AND estado = 'activa'`, idOrg,
	).Scan(&propietariosActivos); err != nil {
		t.Fatalf("contando propietarios activos tras la carrera: %v", err)
	}
	if propietariosActivos != 1 {
		t.Fatalf("tras la carrera de concurrencia hay %d propietario(s) activo(s), se esperaba exactamente 1 (INV-TEN-06)", propietariosActivos)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// --- 5. Invitaciones ----------------------------------------------------------

// invitarMiembroDePrueba invita a correoDestino a idOrganizacion con el rol
// dado, ejecutado por tokenEjecutor, y devuelve el id de la invitación
// creada (VistaInvitacion.ID — nunca el token, que solo sale por el
// notificador, INV-TEN-23).
func invitarMiembroDePrueba(t *testing.T, app *fiber.App, tokenEjecutor, idOrganizacion, correoDestino, rol string) string {
	t.Helper()
	var resultado struct {
		ID string `json:"id"`
	}
	req := peticionJSON(t, http.MethodPost, "/tenencia/organizaciones/"+idOrganizacion+"/invitaciones", map[string]any{
		"correo": correoDestino,
		"rol":    rol,
	})
	req.Header.Set("Authorization", "Bearer "+tokenEjecutor)
	status := respuestaHTTP(t, app, req, &resultado)
	if status != http.StatusCreated {
		t.Fatalf("invitar miembro de prueba (org=%s correo=%s): status=%d", idOrganizacion, correoDestino, status)
	}
	return resultado.ID
}

// TestTenencia_Invitacion_AceptacionPorDestinatarioCorrecto_CreaMembresia
// cubre el camino feliz de invitación: el destinatario correcto, autenticado
// y activo, acepta y queda como miembro con el rol propuesto.
func TestTenencia_Invitacion_AceptacionPorDestinatarioCorrecto_CreaMembresia(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app, captura := nuevoServidorTenencia(t, pool)

	idA, correoA := usuarioActivoDePrueba(t, app, dueno, "inv-ok-a")
	t.Cleanup(func() { borrarUsuario(t, dueno, idA) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idA) })
	tokenA := iniciarSesionDePrueba(t, app, correoA).TokenAcceso

	idB, correoB := usuarioActivoDePrueba(t, app, dueno, "inv-ok-b")
	t.Cleanup(func() { borrarUsuario(t, dueno, idB) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idB) })
	tokenB := iniciarSesionDePrueba(t, app, correoB).TokenAcceso

	idOrg := crearOrganizacionDePrueba(t, app, tokenA, "Organización de Invitación OK", aliasUnico(t, "inv-ok"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrg) })

	invitarMiembroDePrueba(t, app, tokenA, idOrg, correoB, "miembro")
	token := captura.token()
	if token == "" {
		t.Fatalf("el notificador no capturó ningún token de invitación")
	}

	var vistaMiembro struct {
		IDUsuario string `json:"id_usuario"`
		Rol       string `json:"rol"`
		Estado    string `json:"estado"`
	}
	req := peticionJSON(t, http.MethodPost, "/tenencia/invitaciones/aceptaciones", map[string]any{"token": token})
	req.Header.Set("Authorization", "Bearer "+tokenB)
	if status := respuestaHTTP(t, app, req, &vistaMiembro); status != http.StatusCreated {
		t.Fatalf("aceptar invitación con el destinatario correcto: status=%d, se esperaba 201", status)
	}
	if vistaMiembro.IDUsuario != idB {
		t.Fatalf("id_usuario de la membresía resultante = %q, se esperaba %q", vistaMiembro.IDUsuario, idB)
	}
	if vistaMiembro.Rol != "miembro" {
		t.Fatalf("rol de la membresía resultante = %q, se esperaba miembro", vistaMiembro.Rol)
	}
	if vistaMiembro.Estado != "activa" {
		t.Fatalf("estado de la membresía resultante = %q, se esperaba activa", vistaMiembro.Estado)
	}

	// B ahora puede ver la organización.
	reqVer := peticionJSON(t, http.MethodGet, "/tenencia/organizaciones/"+idOrg, nil)
	reqVer.Header.Set("Authorization", "Bearer "+tokenB)
	if status := respuestaHTTP(t, app, reqVer, nil); status != http.StatusOK {
		t.Fatalf("GET organización tras aceptar la invitación: status=%d, se esperaba 200", status)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// TestTenencia_Invitacion_DestinatarioIncorrecto_FallaComoTokenInvalido
// cubre INV-TEN-21/INV-TEN-24: un tercero autenticado, cuyo correo NO
// coincide con el destinatario de la invitación, recibe la MISMA respuesta
// observable (404) que un token inválido — nunca un error que delate que el
// token en sí era válido mas el destinatario no coincidía.
func TestTenencia_Invitacion_DestinatarioIncorrecto_FallaComoTokenInvalido(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app, captura := nuevoServidorTenencia(t, pool)

	idA, correoA := usuarioActivoDePrueba(t, app, dueno, "inv-ajena-a")
	t.Cleanup(func() { borrarUsuario(t, dueno, idA) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idA) })
	tokenA := iniciarSesionDePrueba(t, app, correoA).TokenAcceso

	idB, correoB := usuarioActivoDePrueba(t, app, dueno, "inv-ajena-b")
	t.Cleanup(func() { borrarUsuario(t, dueno, idB) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idB) })

	idC, correoC := usuarioActivoDePrueba(t, app, dueno, "inv-ajena-c")
	t.Cleanup(func() { borrarUsuario(t, dueno, idC) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idC) })
	tokenC := iniciarSesionDePrueba(t, app, correoC).TokenAcceso

	idOrg := crearOrganizacionDePrueba(t, app, tokenA, "Organización de Invitación Ajena", aliasUnico(t, "inv-ajena"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrg) })

	// La invitación es para B, pero C (un tercero no relacionado, también
	// autenticado y activo) intenta redimirla con su propio token de acceso.
	invitarMiembroDePrueba(t, app, tokenA, idOrg, correoB, "miembro")
	token := captura.token()
	if token == "" {
		t.Fatalf("el notificador no capturó ningún token de invitación")
	}

	var errResp errorHumaRespuesta
	req := peticionJSON(t, http.MethodPost, "/tenencia/invitaciones/aceptaciones", map[string]any{"token": token})
	req.Header.Set("Authorization", "Bearer "+tokenC)
	status := respuestaHTTP(t, app, req, &errResp)
	if status != http.StatusNotFound {
		t.Fatalf("aceptar invitación con el destinatario incorrecto: status=%d, se esperaba 404 (INV-TEN-24)", status)
	}
	if errResp.Detail != "invitación inválida" {
		t.Fatalf("detalle del error = %q, se esperaba el mismo mensaje que un token inválido (INV-TEN-24)", errResp.Detail)
	}

	// C nunca quedó como miembro.
	var totalMembresiaC int
	if err := dueno.QueryRow(context.Background(),
		`SELECT count(*) FROM membresias WHERE organizacion_id = $1 AND usuario_id = $2`, idOrg, idC,
	).Scan(&totalMembresiaC); err != nil {
		t.Fatalf("verificando ausencia de membresía de C: %v", err)
	}
	if totalMembresiaC != 0 {
		t.Fatalf("C quedó con una membresía en la organización pese a que su correo no coincidía con el destinatario")
	}
}

// TestTenencia_Invitacion_Expirada_Falla cubre: una invitación cuya ventana
// de vigencia ya pasó no puede aceptarse, con la misma respuesta observable
// que un token inválido (INV-TEN-24).
func TestTenencia_Invitacion_Expirada_Falla(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app, captura := nuevoServidorTenencia(t, pool)

	idA, correoA := usuarioActivoDePrueba(t, app, dueno, "inv-exp-a")
	t.Cleanup(func() { borrarUsuario(t, dueno, idA) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idA) })
	tokenA := iniciarSesionDePrueba(t, app, correoA).TokenAcceso

	idB, correoB := usuarioActivoDePrueba(t, app, dueno, "inv-exp-b")
	t.Cleanup(func() { borrarUsuario(t, dueno, idB) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idB) })
	tokenB := iniciarSesionDePrueba(t, app, correoB).TokenAcceso

	idOrg := crearOrganizacionDePrueba(t, app, tokenA, "Organización de Invitación Expirada", aliasUnico(t, "inv-exp"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrg) })

	idInvitacion := invitarMiembroDePrueba(t, app, tokenA, idOrg, correoB, "miembro")
	token := captura.token()
	if token == "" {
		t.Fatalf("el notificador no capturó ningún token de invitación")
	}

	// Forzar la expiración directamente en BD (rol dueño, bypassa RLS): sin
	// esto habría que esperar los 7 días de PoliticaOrganizacionPorDefecto.
	// Hay que retrasar TAMBIÉN creada_en (no solo expira_en): el CHECK
	// invitaciones_ventana_coherente exige expira_en > creada_en, y
	// creada_en quedó fijada a "ahora" al crear la invitación hace un
	// instante.
	if _, err := dueno.Exec(context.Background(),
		`UPDATE invitaciones SET creada_en = now() - interval '2 hours', expira_en = now() - interval '1 hour' WHERE id = $1`, idInvitacion,
	); err != nil {
		t.Fatalf("forzando la expiración de la invitación: %v", err)
	}

	var errResp errorHumaRespuesta
	req := peticionJSON(t, http.MethodPost, "/tenencia/invitaciones/aceptaciones", map[string]any{"token": token})
	req.Header.Set("Authorization", "Bearer "+tokenB)
	status := respuestaHTTP(t, app, req, &errResp)
	if status != http.StatusNotFound {
		t.Fatalf("aceptar invitación expirada: status=%d, se esperaba 404 (INV-TEN-24)", status)
	}
}

// TestTenencia_Invitacion_Reinvitar_RevocaLaAnterior cubre INV-TEN-22:
// invitar de nuevo al mismo (organización, correo) revoca la invitación
// pendiente anterior en vez de fallar, y deja exactamente una fila
// 'pendiente' en la base.
func TestTenencia_Invitacion_Reinvitar_RevocaLaAnterior(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app, _ := nuevoServidorTenencia(t, pool)

	idA, correoA := usuarioActivoDePrueba(t, app, dueno, "inv-re-a")
	t.Cleanup(func() { borrarUsuario(t, dueno, idA) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idA) })
	tokenA := iniciarSesionDePrueba(t, app, correoA).TokenAcceso

	idB, correoB := usuarioActivoDePrueba(t, app, dueno, "inv-re-b")
	t.Cleanup(func() { borrarUsuario(t, dueno, idB) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idB) })

	idOrg := crearOrganizacionDePrueba(t, app, tokenA, "Organización de Reinvitación", aliasUnico(t, "inv-re"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrg) })

	idInvitacion1 := invitarMiembroDePrueba(t, app, tokenA, idOrg, correoB, "miembro")
	idInvitacion2 := invitarMiembroDePrueba(t, app, tokenA, idOrg, correoB, "administrador")

	if idInvitacion1 == idInvitacion2 {
		t.Fatalf("reinvitar devolvió el mismo id de invitación: se esperaba una fila nueva")
	}

	rows, err := dueno.Query(context.Background(),
		`SELECT id, estado FROM invitaciones WHERE organizacion_id = $1 ORDER BY creada_en`, idOrg,
	)
	if err != nil {
		t.Fatalf("consultando invitaciones tras reinvitar: %v", err)
	}
	defer rows.Close()

	type filaInvitacion struct {
		id     string
		estado string
	}
	var filas []filaInvitacion
	for rows.Next() {
		var f filaInvitacion
		if err := rows.Scan(&f.id, &f.estado); err != nil {
			t.Fatalf("escaneando invitación: %v", err)
		}
		filas = append(filas, f)
	}
	if len(filas) != 2 {
		t.Fatalf("se esperaban 2 filas de invitación (la revocada y la nueva pendiente), hubo %d", len(filas))
	}

	var pendientes, revocadas int
	for _, f := range filas {
		switch {
		case f.id == idInvitacion1 && f.estado == "revocada":
			revocadas++
		case f.id == idInvitacion2 && f.estado == "pendiente":
			pendientes++
		}
	}
	if pendientes != 1 || revocadas != 1 {
		t.Fatalf("estado inesperado tras reinvitar: filas=%+v (se esperaba invitación1=revocada, invitación2=pendiente)", filas)
	}

	assertCadenaAuditoriaIntegra(t, dueno)
}

// --- 6. Auditoría con organizacion_id poblado --------------------------------

// TestTenencia_Auditoria_CadenaIntegraYOrganizacionIDPoblado verifica que,
// tras ejercitar Tenencia, la cadena de hashes de auditoría sigue íntegra
// (mismo mecanismo que Identidad y Acceso) y que las filas de auditoría
// generadas por Tenencia llevan organizacion_id poblado — a diferencia de
// Identidad/Acceso, que siempre lo dejan NULL (INV-TEN-26, la migración
// 000002 documentaba la columna como "NULL hasta que exista Tenencia").
func TestTenencia_Auditoria_CadenaIntegraYOrganizacionIDPoblado(t *testing.T) {
	pool := poolAplicacion(t)
	dueno := poolDueno(t)
	app, _ := nuevoServidorTenencia(t, pool)

	idA, correoA := usuarioActivoDePrueba(t, app, dueno, "aud-a")
	t.Cleanup(func() { borrarUsuario(t, dueno, idA) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idA) })
	tokenA := iniciarSesionDePrueba(t, app, correoA).TokenAcceso

	idB, _ := usuarioActivoDePrueba(t, app, dueno, "aud-b")
	t.Cleanup(func() { borrarUsuario(t, dueno, idB) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idB) })

	idOrg := crearOrganizacionDePrueba(t, app, tokenA, "Organización de Auditoría", aliasUnico(t, "aud"))
	t.Cleanup(func() { borrarOrganizacion(t, dueno, idOrg) })

	agregarMiembroDePrueba(t, app, tokenA, idOrg, idB, "miembro")

	// La organización SIN membresía intenta ser vista por un tercero: genera
	// autorizacion.denegada, también con organizacion_id poblado.
	idC, correoC := usuarioActivoDePrueba(t, app, dueno, "aud-c")
	t.Cleanup(func() { borrarUsuario(t, dueno, idC) })
	t.Cleanup(func() { limpiarSesionesDeUsuario(t, dueno, idC) })
	tokenC := iniciarSesionDePrueba(t, app, correoC).TokenAcceso
	reqDenegado := peticionJSON(t, http.MethodGet, "/tenencia/organizaciones/"+idOrg+"/miembros", nil)
	reqDenegado.Header.Set("Authorization", "Bearer "+tokenC)
	_ = respuestaHTTP(t, app, reqDenegado, nil)

	assertCadenaAuditoriaIntegra(t, dueno)

	var totalConOrganizacion int
	if err := dueno.QueryRow(context.Background(),
		`SELECT count(*) FROM auditoria WHERE organizacion_id = $1`, idOrg,
	).Scan(&totalConOrganizacion); err != nil {
		t.Fatalf("consultando auditoria.organizacion_id: %v", err)
	}
	if totalConOrganizacion == 0 {
		t.Fatalf("ninguna fila de auditoria quedó con organizacion_id = %q; INV-TEN-26 no se está cumpliendo", idOrg)
	}

	var accionesDistintas int
	if err := dueno.QueryRow(context.Background(),
		`SELECT count(DISTINCT accion) FROM auditoria WHERE organizacion_id = $1`, idOrg,
	).Scan(&accionesDistintas); err != nil {
		t.Fatalf("consultando acciones distintas de auditoria: %v", err)
	}
	if accionesDistintas < 2 {
		t.Fatalf("se esperaban al menos 2 acciones distintas de auditoría con organizacion_id poblado "+
			"(p. ej. organizacion.creada, membresia.creada, autorizacion.denegada), hubo %d", accionesDistintas)
	}
}
