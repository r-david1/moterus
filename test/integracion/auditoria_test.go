package integracion

import (
	"net/http"
	"testing"
)

// TestAuditoria_FlujoHTTPCompleto_GeneraAccionesEsperadasYCadenaIntegra
// ejercita registro -> login fallido (contraseña incorrecta) -> login
// exitoso a través de HTTP (la misma pila que
// TestHTTP_Login_Exitoso_DevuelveResultadoSinToken) y después inspecciona
// directamente la tabla auditoria (con el rol dueño, para leer sin
// restricciones) para verificar:
//  1. que cada acción del catálogo cerrado (docs/catalogos/acciones-auditoria.md)
//     se registró con la accion/resultado correctos, y
//  2. que verificar_cadena_auditoria() sigue sin discrepancias después de
//     los inserts (el hash-chaining de la migración 000002 sigue íntegro).
func TestAuditoria_FlujoHTTPCompleto_GeneraAccionesEsperadasYCadenaIntegra(t *testing.T) {
	pool := poolAplicacion(t)
	duenoPool := poolDueno(t)
	app := nuevoServidorIdentidad(t, pool)

	correo := correoUnico(t, "auditoria-flujo-completo")

	// 1. Registro -> usuario.registrado / exito.
	var registro registroRespuestaPrueba
	statusRegistro := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/usuarios", registroPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), &registro)
	t.Cleanup(func() { borrarUsuario(t, duenoPool, registro.IDUsuario) })
	if statusRegistro != http.StatusOK {
		t.Fatalf("el registro (setup) = %d, esperado %d", statusRegistro, http.StatusOK)
	}

	// 2. Login antes de verificar correo -> usuario.login / fallo.
	statusLoginNoVerificado := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), nil)
	if statusLoginNoVerificado != http.StatusForbidden {
		t.Fatalf("login antes de verificar (setup) = %d, esperado %d", statusLoginNoVerificado, http.StatusForbidden)
	}

	activarUsuario(t, duenoPool, registro.IDUsuario)

	// 3. Login con contraseña incorrecta -> usuario.login / fallo.
	statusContrasenaIncorrecta := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correo, Contrasena: "definitivamente-no-es-la-contrasena",
	}), nil)
	if statusContrasenaIncorrecta != http.StatusUnauthorized {
		t.Fatalf("login con contraseña incorrecta (setup) = %d, esperado %d", statusContrasenaIncorrecta, http.StatusUnauthorized)
	}

	// 4. Login exitoso -> usuario.login / exito.
	statusLoginExitoso := respuestaHTTP(t, app, peticionJSON(t, http.MethodPost, "/identidad/autenticaciones", autenticarPeticionPrueba{
		Correo: correo, Contrasena: contrasenaFuerteDePrueba,
	}), nil)
	if statusLoginExitoso != http.StatusOK {
		t.Fatalf("login exitoso (setup) = %d, esperado %d", statusLoginExitoso, http.StatusOK)
	}

	// Ya NO hay un paso 5 de "consulta por un tercero" aquí: desde el cierre
	// de la autorización cruzada de GET /identidad/usuarios/{id} (§11.2 del
	// diseño de Tenencia, docs/design/tenencia-bounded-context.md), el
	// query param `solicitante_id` dejó de tener efecto — es un cambio de
	// contrato deliberado (ver internal/identidad/README.md): quién
	// pregunta sale siempre del token Bearer ya validado, nunca de un
	// parámetro que el cliente controla. nuevoServidorIdentidad monta
	// Identidad SIN un acceso/puertos.ValidadorDeAccesos (ver su firma),
	// así que este archivo, acotado a Identidad en aislamiento, no puede
	// fabricar un Bearer real para ejercitar el camino de tercero
	// autorizado por Tenencia; esa cobertura, con la pila completa
	// (Identidad+Acceso+Tenencia), es responsabilidad de los tests de
	// integración de Tenencia. Lo que sigue vigente y se verifica aquí es
	// la dependencia dura documentada en §11.2: ObtenerUsuario NUNCA audita
	// cuando IDSolicitante llega vacío (llamada interna/no autenticada) —
	// ver TestObtenerUsuarioCasoDeUso_ConsultaInterna_IDSolicitanteVacio_NoAudita
	// en internal/identidad/aplicacion/obtener_usuario_test.go (ya cubierta,
	// sin cambios: Acceso y ahora Tenencia dependen de este comportamiento).

	// --- verificación directa de la bitácora (rol dueño, sin restricciones) ---

	// Casts explícitos: recurso_id es TEXT y usuario_id es UUID, y con el
	// mismo parámetro posicional en ambos lados Postgres no puede inferir
	// un único tipo para $1 ("operator does not exist: uuid = text").
	filas, err := duenoPool.Query(t.Context(),
		`SELECT accion, resultado FROM auditoria WHERE recurso_id = $1::text OR usuario_id = $1::uuid ORDER BY secuencia`,
		registro.IDUsuario,
	)
	if err != nil {
		t.Fatalf("consultando auditoria: %v", err)
	}
	type filaAuditoria struct{ accion, resultado string }
	var encontradas []filaAuditoria
	for filas.Next() {
		var f filaAuditoria
		if err := filas.Scan(&f.accion, &f.resultado); err != nil {
			t.Fatalf("escaneando fila de auditoria: %v", err)
		}
		encontradas = append(encontradas, f)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		t.Fatalf("iterando auditoria: %v", err)
	}

	esperadas := []filaAuditoria{
		{"usuario.registrado", "exito"},
		{"usuario.login", "fallo"}, // correo no verificado
		{"usuario.login", "fallo"}, // contraseña incorrecta
		{"usuario.login", "exito"},
	}
	if len(encontradas) != len(esperadas) {
		t.Fatalf("se encontraron %d filas de auditoría, esperadas %d: %+v", len(encontradas), len(esperadas), encontradas)
	}
	for i, esperada := range esperadas {
		if encontradas[i] != esperada {
			t.Errorf("fila de auditoría #%d = %+v, esperada %+v (orden completo encontrado: %+v)", i, encontradas[i], esperada, encontradas)
		}
	}

	// La cadena de hashes sigue íntegra tras todos estos inserts.
	assertCadenaAuditoriaIntegra(t, duenoPool)
}
