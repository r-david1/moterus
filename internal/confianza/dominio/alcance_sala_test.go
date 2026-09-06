package dominio

import (
	"errors"
	"testing"
)

func idOrganizacionDePrueba(t *testing.T) IDOrganizacion {
	t.Helper()
	id, err := IDOrganizacionDesde("018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	return id
}

func TestAlcanceSistema_ConstruyeSinOrganizacion(t *testing.T) {
	a := AlcanceSistema()
	if !a.EsSistema() {
		t.Error("EsSistema() debería ser true")
	}
	if _, ok := a.OrganizacionID(); ok {
		t.Error("un alcance sistema no debería tener IDOrganizacion")
	}
	if a.EsVacio() {
		t.Error("no debería estar vacío")
	}
}

func TestAlcanceOrganizacion_ExigeIDOrganizacionNoVacio(t *testing.T) {
	if _, err := AlcanceOrganizacion(IDOrganizacion{}); err == nil {
		t.Fatal("se esperaba error con IDOrganizacion vacío")
	} else {
		var errAlcance *ErrAlcanceSalaInvalido
		if !errors.As(err, &errAlcance) {
			t.Errorf("se esperaba *ErrAlcanceSalaInvalido, obtuvo %T", err)
		}
	}
}

func TestAlcanceOrganizacion_Construye(t *testing.T) {
	idOrg := idOrganizacionDePrueba(t)
	a, err := AlcanceOrganizacion(idOrg)
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if a.EsSistema() {
		t.Error("EsSistema() debería ser false")
	}
	id, ok := a.OrganizacionID()
	if !ok || !id.EsIgual(idOrg) {
		t.Error("OrganizacionID() no coincide con el id provisto")
	}
}

func TestAlcanceSala_Clave(t *testing.T) {
	idOrg := idOrganizacionDePrueba(t)
	org, _ := AlcanceOrganizacion(idOrg)
	if AlcanceSistema().Clave() != "sistema" {
		t.Errorf("Clave() = %q, esperado \"sistema\"", AlcanceSistema().Clave())
	}
	esperado := "org:" + idOrg.String()
	if org.Clave() != esperado {
		t.Errorf("Clave() = %q, esperado %q", org.Clave(), esperado)
	}
}

func TestAlcanceSala_EsIgual(t *testing.T) {
	idOrg := idOrganizacionDePrueba(t)
	org1, _ := AlcanceOrganizacion(idOrg)
	org2, _ := AlcanceOrganizacion(idOrg)
	if !org1.EsIgual(org2) {
		t.Error("dos alcances organizacion con el mismo id deberían ser iguales")
	}
	if org1.EsIgual(AlcanceSistema()) {
		t.Error("un alcance organizacion no debería ser igual a uno sistema")
	}
}

func TestReconstituirAlcanceSala_ExigeCoherencia(t *testing.T) {
	if _, err := ReconstituirAlcanceSala("sistema", "018e6f2a-9c3d-7c3a-8b3a-1e2f3a4b5c6d"); err == nil {
		t.Error("un alcance sistema con organizacionID no vacío debería fallar")
	}
	if _, err := ReconstituirAlcanceSala("organizacion", ""); err == nil {
		t.Error("un alcance organizacion sin organizacionID debería fallar")
	}
	if _, err := ReconstituirAlcanceSala("otro", ""); err == nil {
		t.Error("un tipo fuera del catálogo debería fallar")
	}

	a, err := ReconstituirAlcanceSala("sistema", "")
	if err != nil || !a.EsSistema() {
		t.Errorf("ReconstituirAlcanceSala(sistema, \"\") = %v, %v", a, err)
	}
	idOrg := idOrganizacionDePrueba(t)
	b, err := ReconstituirAlcanceSala("organizacion", idOrg.String())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	id, ok := b.OrganizacionID()
	if !ok || !id.EsIgual(idOrg) {
		t.Error("OrganizacionID() no coincide tras reconstituir")
	}
}

// --- RutaProtegida -----------------------------------------------------

func TestRutaProtegidaDesde_CatalogoCerrado(t *testing.T) {
	validas := []RutaProtegida{RutaAccesoIniciarSesion, RutaIdentidadRegistrarUsuario, RutaTenenciaAceptarInvitacion}
	for _, v := range validas {
		r, err := RutaProtegidaDesde(v.String())
		if err != nil {
			t.Errorf("RutaProtegidaDesde(%q) = error %v, no se esperaba", v.String(), err)
		}
		if !r.EsIgual(v) {
			t.Errorf("RutaProtegidaDesde(%q) = %v, esperado %v", v.String(), r, v)
		}
	}
}

// TestINV_COLA_10_CatalogoCerradoExcluyeRutasDeCredencialCorta verifica que
// INV-COLA-10 se cumple por construcción del catálogo cerrado (§1.6 y §4
// del diseño): ninguna de las rutas explícitamente excluidas —renovación de
// sesión (encolarla expulsaría a usuarios ya autenticados), segundo factor
// (el token de step-up vive 5 minutos sin refresco, INV-MFA-04) ni el
// endpoint JWKS— puede protegerse, porque RutaProtegidaDesde las rechaza:
// no pertenecen al catálogo. No hace falta comparar duraciones en runtime
// porque el catálogo cerrado ya decide la pregunta.
func TestINV_COLA_10_CatalogoCerradoExcluyeRutasDeCredencialCorta(t *testing.T) {
	excluidas := []string{
		"acceso.renovar_sesion",
		"acceso.completar_segundo_factor",
		"identidad.jwks",
		"",
	}
	for _, v := range excluidas {
		if _, err := RutaProtegidaDesde(v); err == nil {
			t.Errorf("RutaProtegidaDesde(%q) = nil, se esperaba ErrRutaNoProtegible (INV-COLA-10)", v)
		} else {
			var errRuta *ErrRutaNoProtegible
			if !errors.As(err, &errRuta) {
				t.Errorf("se esperaba *ErrRutaNoProtegible, obtuvo %T", err)
			}
		}
	}
	if len(validas3()) != 3 {
		t.Fatalf("el catálogo cerrado debe tener exactamente 3 rutas, tiene %d", len(validas3()))
	}
}

func validas3() []RutaProtegida {
	return []RutaProtegida{RutaAccesoIniciarSesion, RutaIdentidadRegistrarUsuario, RutaTenenciaAceptarInvitacion}
}

// TestINV_COLA_09_RutasDeCatalogo_SoloAdmitenAlcanceSistema verifica la
// precondición de dominio de la que depende el orden de la cadena de
// middlewares (INV-COLA-09, §1.6 y §7.3 del diseño): las tres rutas del
// catálogo actual son pre-autenticación y ninguna admite un AlcanceSala
// organizacion — su clave nunca depende de un {idOrganizacion}, así que
// siempre pueden evaluarse ANTES de cualquier autenticación/autorización,
// nunca después.
func TestINV_COLA_09_RutasDeCatalogo_SoloAdmitenAlcanceSistema(t *testing.T) {
	org, _ := AlcanceOrganizacion(idOrganizacionDePrueba(t))
	for _, r := range validas3() {
		if !r.AdmiteAlcance(AlcanceSistema()) {
			t.Errorf("%s debería admitir AlcanceSistema", r.String())
		}
		if r.AdmiteAlcance(org) {
			t.Errorf("%s no debería admitir un AlcanceSala organizacion (INV-COLA-09)", r.String())
		}
	}
}

// TestRutaProtegida_VidaDeLaCredencialDeEntrada documenta que, para el
// catálogo actual de rutas pre-autenticación, el valor es 0 ("sin
// restricción activa conocida"): INV-COLA-10 se cumple hoy por la clausura
// del catálogo (ver TestINV_COLA_10), no por esta comparación de duración.
func TestRutaProtegida_VidaDeLaCredencialDeEntrada(t *testing.T) {
	for _, r := range validas3() {
		if r.VidaDeLaCredencialDeEntrada() != 0 {
			t.Errorf("%s: VidaDeLaCredencialDeEntrada() = %v, esperado 0", r.String(), r.VidaDeLaCredencialDeEntrada())
		}
	}
}

// TestRutaProtegida_AdmiteAlcance_RutaFueraDelCatalogo verifica la rama por
// defecto de AdmiteAlcance: una RutaProtegida zero value (o cualquier valor
// fuera del catálogo cerrado, que solo podría llegar aquí por un bug de
// otro paquete de este mismo módulo, nunca por un dato externo — el
// dominio los rechaza antes en RutaProtegidaDesde) no admite ningún
// alcance.
func TestRutaProtegida_AdmiteAlcance_RutaFueraDelCatalogo(t *testing.T) {
	var r RutaProtegida
	if r.AdmiteAlcance(AlcanceSistema()) {
		t.Error("una ruta fuera del catálogo no debería admitir ningún alcance")
	}
}

func TestRutaProtegida_EsIgualYEsVacio(t *testing.T) {
	var r RutaProtegida
	if !r.EsVacio() {
		t.Error("el zero value debería estar vacío")
	}
	if !RutaAccesoIniciarSesion.EsIgual(RutaAccesoIniciarSesion) {
		t.Error("una ruta debería ser igual a sí misma")
	}
	if RutaAccesoIniciarSesion.EsIgual(RutaIdentidadRegistrarUsuario) {
		t.Error("rutas distintas no deberían ser iguales")
	}
}

// --- ClaveSala -----------------------------------------------------------

// TestINV_COLA_01_ClaveSala_EsElDiscriminadorDeUnicidad verifica la mitad
// de dominio de INV-COLA-01 ("a lo sumo una sala no cerrada por par
// (alcance, ruta)"): dos salas con el mismo (alcance, ruta) producen la
// MISMA ClaveSala, y salas con distinto alcance o distinta ruta producen
// claves distintas. Es precisamente el valor sobre el que el índice único
// parcial de Postgres (§6.1 del diseño) aplicaría su restricción — la
// garantía estructural de unicidad en sí misma vive en Postgres, fuera del
// alcance de un test de dominio puro.
func TestINV_COLA_01_ClaveSala_EsElDiscriminadorDeUnicidad(t *testing.T) {
	idOrg := idOrganizacionDePrueba(t)
	org, _ := AlcanceOrganizacion(idOrg)

	c1 := nuevaClaveSala(AlcanceSistema(), RutaAccesoIniciarSesion)
	c2 := nuevaClaveSala(AlcanceSistema(), RutaAccesoIniciarSesion)
	if !c1.EsIgual(c2) {
		t.Error("el mismo (alcance, ruta) debe producir la misma ClaveSala")
	}

	c3 := nuevaClaveSala(AlcanceSistema(), RutaIdentidadRegistrarUsuario)
	if c1.EsIgual(c3) {
		t.Error("distinta ruta debe producir distinta ClaveSala")
	}

	c4 := nuevaClaveSala(org, RutaAccesoIniciarSesion)
	if c1.EsIgual(c4) {
		t.Error("distinto alcance debe producir distinta ClaveSala")
	}
	if c4.String() != "org:"+idOrg.String()+":"+RutaAccesoIniciarSesion.String() {
		t.Errorf("ClaveSala.String() = %q", c4.String())
	}
}
