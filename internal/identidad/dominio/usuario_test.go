package dominio

import (
	"errors"
	"testing"
	"time"
)

func idDePrueba(t *testing.T) IDUsuario {
	t.Helper()
	id, err := IDUsuarioDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if err != nil {
		t.Fatalf("no se pudo construir el IDUsuario de prueba: %v", err)
	}
	return id
}

func hashDePrueba(t *testing.T) HashContrasena {
	t.Helper()
	h, err := NuevoHashContrasena(hashValido)
	if err != nil {
		t.Fatalf("no se pudo construir el hash de prueba: %v", err)
	}
	return h
}

func usuarioDePrueba(t *testing.T) (*Usuario, time.Time) {
	t.Helper()
	ahora := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	u, err := RegistrarUsuario(idDePrueba(t), correoDePrueba(t, "usuario@ejemplo.com"), hashDePrueba(t), ahora)
	if err != nil {
		t.Fatalf("no se pudo registrar el usuario de prueba: %v", err)
	}
	return u, ahora
}

// TestINV_ID_07_NaceEnPendienteVerificacion verifica que un usuario recién
// registrado nace en pendiente_verificacion, nunca en activo.
func TestINV_ID_07_NaceEnPendienteVerificacion(t *testing.T) {
	u, _ := usuarioDePrueba(t)
	if !u.Estado().EsIgual(EstadoPendienteVerificacion) {
		t.Errorf("Estado() = %s, esperado pendiente_verificacion", u.Estado())
	}
}

// TestINV_ID_01_RequiereCorreoYHashValidos verifica que no puede existir un
// Usuario sin credencial: RegistrarUsuario falla si el hash es inválido.
func TestINV_ID_01_RequiereCorreoYHashValidos(t *testing.T) {
	var hashVacio HashContrasena
	_, err := RegistrarUsuario(idDePrueba(t), correoDePrueba(t, "a@b.com"), hashVacio, time.Now())
	if err == nil {
		t.Fatal("se esperaba error al registrar un usuario con hash vacío")
	}
	var errCredencial *ErrCredencialInvalida
	if !errors.As(err, &errCredencial) {
		t.Errorf("se esperaba *ErrCredencialInvalida, obtuvo %T", err)
	}
}

func TestRegistrarUsuario_RechazaIDVacio(t *testing.T) {
	var idVacio IDUsuario
	_, err := RegistrarUsuario(idVacio, correoDePrueba(t, "a@b.com"), hashDePrueba(t), time.Now())
	if err == nil {
		t.Fatal("se esperaba error al registrar un usuario con ID vacío")
	}
}

func TestRegistrarUsuario_AcumulaEventoUsuarioRegistrado(t *testing.T) {
	u, ahora := usuarioDePrueba(t)
	eventos := u.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento acumulado, hay %d", len(eventos))
	}
	ev, ok := eventos[0].(UsuarioRegistrado)
	if !ok {
		t.Fatalf("se esperaba un evento UsuarioRegistrado, obtuvo %T", eventos[0])
	}
	if ev.CorreoNormalizado != "usuario@ejemplo.com" {
		t.Errorf("CorreoNormalizado = %q, esperado usuario@ejemplo.com", ev.CorreoNormalizado)
	}
	if !ev.OcurridoEn().Equal(ahora) {
		t.Errorf("OcurridoEn() = %v, esperado %v", ev.OcurridoEn(), ahora)
	}
}

// TestEventosPendientes_Drena verifica que EventosPendientes vacía el
// buffer interno tras devolver los eventos acumulados.
func TestEventosPendientes_Drena(t *testing.T) {
	u, _ := usuarioDePrueba(t)
	primero := u.EventosPendientes()
	if len(primero) == 0 {
		t.Fatal("se esperaba al menos un evento en la primera llamada")
	}
	segundo := u.EventosPendientes()
	if len(segundo) != 0 {
		t.Errorf("la segunda llamada a EventosPendientes debe devolver un slice vacío, obtuvo %d elementos", len(segundo))
	}
}

// --- VerificarContrasena ----------------------------------------------------

type verificadorFalso struct {
	resultado bool
	llamado   bool
}

func (v *verificadorFalso) Verificar(hash HashContrasena, plana ContrasenaPlana) bool {
	v.llamado = true
	return v.resultado
}

func TestUsuario_VerificarContrasena(t *testing.T) {
	u, _ := usuarioDePrueba(t)
	plana, _ := NuevaContrasenaPlana("cualquiera")

	v := &verificadorFalso{resultado: true}
	if !u.VerificarContrasena(v, plana) {
		t.Error("se esperaba true cuando el verificador reporta éxito")
	}
	if !v.llamado {
		t.Error("VerificarContrasena debe delegar en el VerificadorContrasenas inyectado")
	}

	vFalso := &verificadorFalso{resultado: false}
	if u.VerificarContrasena(vFalso, plana) {
		t.Error("se esperaba false cuando el verificador reporta fallo")
	}
}

func TestUsuario_VerificarContrasena_NilSeguro(t *testing.T) {
	u, _ := usuarioDePrueba(t)
	plana, _ := NuevaContrasenaPlana("cualquiera")
	if u.VerificarContrasena(nil, plana) {
		t.Error("un verificador nil debe resultar en false, no en panic")
	}
}

// --- CambiarContrasena / ReemplazarHash -------------------------------------

func TestUsuario_CambiarContrasena(t *testing.T) {
	u, ahora := usuarioDePrueba(t)
	u.EventosPendientes() // drenar el evento de registro

	nuevoHash, _ := NuevoHashContrasena("$argon2id$v=19$m=65536,t=3,p=1$otra$otrocosaotra")
	despues := ahora.Add(time.Hour)
	if err := u.CambiarContrasena(nuevoHash, despues); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if u.Credencial().Hash().Valor() != nuevoHash.Valor() {
		t.Error("el hash debe haberse reemplazado")
	}
	if !u.ActualizadoEn().Equal(despues) {
		t.Errorf("ActualizadoEn() = %v, esperado %v (INV-ID-10)", u.ActualizadoEn(), despues)
	}
	eventos := u.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	if _, ok := eventos[0].(ContrasenaCambiada); !ok {
		t.Errorf("se esperaba ContrasenaCambiada, obtuvo %T", eventos[0])
	}
}

func TestUsuario_ReemplazarHash(t *testing.T) {
	u, ahora := usuarioDePrueba(t)
	u.EventosPendientes()

	nuevoHash, _ := NuevoHashContrasena("$argon2id$v=19$m=131072,t=4,p=1$otra$otrocosaotra")
	despues := ahora.Add(time.Minute)
	if err := u.ReemplazarHash(nuevoHash, despues); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	eventos := u.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	if _, ok := eventos[0].(CredencialRehasheada); !ok {
		t.Errorf("se esperaba CredencialRehasheada, obtuvo %T", eventos[0])
	}
}

// --- ConfirmarCorreo ---------------------------------------------------------

func TestUsuario_ConfirmarCorreo_DesdePendienteVerificacion(t *testing.T) {
	u, ahora := usuarioDePrueba(t)
	u.EventosPendientes()

	despues := ahora.Add(time.Hour)
	if err := u.ConfirmarCorreo(despues); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !u.Estado().EsIgual(EstadoActivo) {
		t.Errorf("Estado() = %s, esperado activo", u.Estado())
	}
	eventos := u.EventosPendientes()
	if len(eventos) != 1 {
		t.Fatalf("se esperaba 1 evento, hay %d", len(eventos))
	}
	if _, ok := eventos[0].(CorreoVerificado); !ok {
		t.Errorf("se esperaba CorreoVerificado, obtuvo %T", eventos[0])
	}
}

func TestUsuario_ConfirmarCorreo_RechazaDesdeOtrosEstados(t *testing.T) {
	estados := []EstadoUsuario{EstadoActivo, EstadoSuspendido, EstadoBloqueado, EstadoAnonimizado}
	for _, estado := range estados {
		u := usuarioEnEstado(t, estado)
		err := u.ConfirmarCorreo(time.Now())
		if err == nil {
			t.Errorf("ConfirmarCorreo desde %s debía fallar", estado)
			continue
		}
		var errTransicion *ErrTransicionEstadoInvalida
		if !errors.As(err, &errTransicion) {
			t.Errorf("se esperaba *ErrTransicionEstadoInvalida, obtuvo %T", err)
		}
	}
}

// --- Suspender ---------------------------------------------------------------

func TestUsuario_Suspender_DesdeActivo(t *testing.T) {
	u := usuarioEnEstado(t, EstadoActivo)
	u.EventosPendientes()
	motivo, _ := NuevoMotivoCambioEstado("actividad sospechosa")
	if err := u.Suspender(motivo, time.Now()); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !u.Estado().EsIgual(EstadoSuspendido) {
		t.Errorf("Estado() = %s, esperado suspendido", u.Estado())
	}
	eventos := u.EventosPendientes()
	ev, ok := eventos[0].(EstadoUsuarioCambiado)
	if !ok {
		t.Fatalf("se esperaba EstadoUsuarioCambiado, obtuvo %T", eventos[0])
	}
	if ev.EstadoAnterior != "activo" || ev.EstadoNuevo != "suspendido" {
		t.Errorf("EstadoAnterior/EstadoNuevo = %s/%s, esperado activo/suspendido", ev.EstadoAnterior, ev.EstadoNuevo)
	}
	if ev.Motivo != "actividad sospechosa" {
		t.Errorf("Motivo = %q, esperado 'actividad sospechosa'", ev.Motivo)
	}
}

func TestUsuario_Suspender_RechazaDesdeOtrosEstados(t *testing.T) {
	estados := []EstadoUsuario{EstadoPendienteVerificacion, EstadoSuspendido, EstadoBloqueado, EstadoAnonimizado}
	motivo, _ := NuevoMotivoCambioEstado("motivo")
	for _, estado := range estados {
		u := usuarioEnEstado(t, estado)
		if err := u.Suspender(motivo, time.Now()); err == nil {
			t.Errorf("Suspender desde %s debía fallar", estado)
		}
	}
}

// --- Bloquear ------------------------------------------------------------

func TestUsuario_Bloquear_DesdeOrigenesPermitidos(t *testing.T) {
	motivo, _ := NuevoMotivoCambioEstado("infracción de términos")
	for _, origen := range []EstadoUsuario{EstadoPendienteVerificacion, EstadoActivo} {
		u := usuarioEnEstado(t, origen)
		u.EventosPendientes()
		if err := u.Bloquear(motivo, time.Now()); err != nil {
			t.Errorf("Bloquear desde %s debía tener éxito: %v", origen, err)
			continue
		}
		if !u.Estado().EsIgual(EstadoBloqueado) {
			t.Errorf("Estado() = %s, esperado bloqueado", u.Estado())
		}
	}
}

func TestUsuario_Bloquear_RechazaDesdeOtrosEstados(t *testing.T) {
	motivo, _ := NuevoMotivoCambioEstado("motivo")
	for _, estado := range []EstadoUsuario{EstadoSuspendido, EstadoBloqueado, EstadoAnonimizado} {
		u := usuarioEnEstado(t, estado)
		if err := u.Bloquear(motivo, time.Now()); err == nil {
			t.Errorf("Bloquear desde %s debía fallar", estado)
		}
	}
}

// --- Reactivar -----------------------------------------------------------

func TestUsuario_Reactivar_DesdeOrigenesPermitidos(t *testing.T) {
	for _, origen := range []EstadoUsuario{EstadoSuspendido, EstadoBloqueado} {
		u := usuarioEnEstado(t, origen)
		u.EventosPendientes()
		if err := u.Reactivar(time.Now()); err != nil {
			t.Errorf("Reactivar desde %s debía tener éxito: %v", origen, err)
			continue
		}
		if !u.Estado().EsIgual(EstadoActivo) {
			t.Errorf("Estado() = %s, esperado activo", u.Estado())
		}
	}
}

func TestUsuario_Reactivar_RechazaDesdeOtrosEstados(t *testing.T) {
	for _, estado := range []EstadoUsuario{EstadoPendienteVerificacion, EstadoActivo, EstadoAnonimizado} {
		u := usuarioEnEstado(t, estado)
		if err := u.Reactivar(time.Now()); err == nil {
			t.Errorf("Reactivar desde %s debía fallar", estado)
		}
	}
}

// TestUsuario_ConfirmarCorreoYReactivar_NoSonIntercambiables es la
// regresión específica del análisis de diseño: aunque tanto
// pendiente_verificacion->activo (confirmar_correo) como
// suspendido/bloqueado->activo (reactivar) comparten el destino "activo",
// cada método solo debe aceptar su propio origen.
func TestUsuario_ConfirmarCorreoYReactivar_NoSonIntercambiables(t *testing.T) {
	suspendido := usuarioEnEstado(t, EstadoSuspendido)
	if err := suspendido.ConfirmarCorreo(time.Now()); err == nil {
		t.Error("ConfirmarCorreo no debe funcionar sobre un usuario suspendido, aunque Reactivar sí")
	}

	pendiente := usuarioEnEstado(t, EstadoPendienteVerificacion)
	if err := pendiente.Reactivar(time.Now()); err == nil {
		t.Error("Reactivar no debe funcionar sobre un usuario pendiente_verificacion, aunque ConfirmarCorreo sí")
	}
}

// --- RegistrarAcceso / PuedeIniciarSesion -----------------------------------

func TestUsuario_RegistrarAcceso(t *testing.T) {
	u, ahora := usuarioDePrueba(t)
	if _, huboAcceso := u.UltimoAccesoEn(); huboAcceso {
		t.Fatal("un usuario recién registrado no debe tener UltimoAccesoEn")
	}
	despues := ahora.Add(24 * time.Hour)
	u.RegistrarAcceso(despues)
	valor, huboAcceso := u.UltimoAccesoEn()
	if !huboAcceso {
		t.Fatal("se esperaba UltimoAccesoEn tras RegistrarAcceso")
	}
	if !valor.Equal(despues) {
		t.Errorf("UltimoAccesoEn() = %v, esperado %v", valor, despues)
	}
	if !u.ActualizadoEn().Equal(despues) {
		t.Errorf("ActualizadoEn() = %v, esperado %v", u.ActualizadoEn(), despues)
	}
}

// TestINV_ID_06_SoloActivoPuedeIniciarSesion verifica que solo el estado
// activo permite iniciar sesión, y que cada otro estado produce el error
// específico correspondiente.
func TestINV_ID_06_SoloActivoPuedeIniciarSesion(t *testing.T) {
	casos := []struct {
		estado    EstadoUsuario
		esperaNil bool
		verificar func(error) bool
	}{
		{EstadoActivo, true, nil},
		{EstadoPendienteVerificacion, false, func(err error) bool {
			var e *ErrCorreoNoVerificado
			return errors.As(err, &e)
		}},
		{EstadoSuspendido, false, func(err error) bool {
			var e *ErrCuentaSuspendida
			return errors.As(err, &e)
		}},
		{EstadoBloqueado, false, func(err error) bool {
			var e *ErrCuentaBloqueada
			return errors.As(err, &e)
		}},
		{EstadoAnonimizado, false, func(err error) bool {
			var e *ErrCuentaBloqueada
			return errors.As(err, &e)
		}},
	}
	for _, c := range casos {
		u := usuarioEnEstado(t, c.estado)
		err := u.PuedeIniciarSesion()
		if c.esperaNil {
			if err != nil {
				t.Errorf("PuedeIniciarSesion() desde %s = %v, esperado nil", c.estado, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("PuedeIniciarSesion() desde %s debía devolver error", c.estado)
			continue
		}
		if !c.verificar(err) {
			t.Errorf("PuedeIniciarSesion() desde %s devolvió un error del tipo incorrecto: %T", c.estado, err)
		}
	}
}

// --- Reconstituir ------------------------------------------------------------

func TestReconstituir_NoAcumulaEventos(t *testing.T) {
	credencial, _ := NuevaCredencial(hashDePrueba(t), time.Now())
	u := Reconstituir(
		idDePrueba(t),
		correoDePrueba(t, "a@b.com"),
		credencial,
		EstadoActivo,
		true,
		time.Now(),
		time.Now(),
		nil,
	)
	if len(u.EventosPendientes()) != 0 {
		t.Error("Reconstituir no debe generar eventos de dominio")
	}
	if !u.TieneMFA() {
		t.Error("TieneMFA() debe reflejar el valor pasado a Reconstituir")
	}
}

func TestReconstituir_UltimoAccesoEn_NoExponePunteroInterno(t *testing.T) {
	credencial, _ := NuevaCredencial(hashDePrueba(t), time.Now())
	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	u := Reconstituir(idDePrueba(t), correoDePrueba(t, "a@b.com"), credencial, EstadoActivo, false, time.Now(), time.Now(), &original)

	// Mutar el puntero original no debe afectar al agregado (INV-ID-09: no
	// se exponen punteros internos, y el constructor debe copiar el valor).
	original = original.Add(time.Hour)

	valor, ok := u.UltimoAccesoEn()
	if !ok {
		t.Fatal("se esperaba UltimoAccesoEn presente")
	}
	if valor.Equal(original) {
		t.Error("Reconstituir debe copiar el valor apuntado, no aliasear el puntero recibido")
	}
}

func TestUsuario_Getters(t *testing.T) {
	u, ahora := usuarioDePrueba(t)
	if !u.ID().EsIgual(idDePrueba(t)) {
		t.Error("ID() debe devolver el identificador con el que se registró el usuario")
	}
	if u.Correo().Normalizado() != "usuario@ejemplo.com" {
		t.Errorf("Correo() = %q, esperado usuario@ejemplo.com", u.Correo().Normalizado())
	}
	if !u.CreadoEn().Equal(ahora) {
		t.Errorf("CreadoEn() = %v, esperado %v", u.CreadoEn(), ahora)
	}
	if u.TieneMFA() {
		t.Error("un usuario recién registrado no debe tener MFA")
	}
}

func TestUsuario_CambiarContrasena_PropagaErrorDeHashInvalido(t *testing.T) {
	u, _ := usuarioDePrueba(t)
	var hashVacio HashContrasena
	if err := u.CambiarContrasena(hashVacio, time.Now()); err == nil {
		t.Error("se esperaba que CambiarContrasena propagara el error de NuevaCredencial")
	}
}

func TestUsuario_ReemplazarHash_PropagaErrorDeHashInvalido(t *testing.T) {
	u, _ := usuarioDePrueba(t)
	var hashVacio HashContrasena
	if err := u.ReemplazarHash(hashVacio, time.Now()); err == nil {
		t.Error("se esperaba que ReemplazarHash propagara el error de NuevaCredencial")
	}
}

// --- helpers ------------------------------------------------------------

// usuarioEnEstado construye un usuario y lo lleva forzosamente al estado
// indicado usando Reconstituir (no las transiciones de negocio), para poder
// probar cada método de transición de forma aislada desde cualquier
// origen, incluidos los que deberían ser rechazados.
func usuarioEnEstado(t *testing.T, estado EstadoUsuario) *Usuario {
	t.Helper()
	credencial, err := NuevaCredencial(hashDePrueba(t), time.Now())
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	return Reconstituir(
		idDePrueba(t),
		correoDePrueba(t, "usuario@ejemplo.com"),
		credencial,
		estado,
		false,
		time.Now(),
		time.Now(),
		nil,
	)
}
