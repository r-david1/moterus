package aplicacion_test

// Fixtures compartidos entre los tests de los casos de uso de aplicacion.
// Viven en package aplicacion_test (caja negra): los tests solo ejercen la
// API pública de aplicacion/puertos/dominio, tal como lo haría un
// consumidor real (p. ej. el adaptador HTTP). Mismo patrón que
// acceso/aplicacion/helpers_aplicacion_test.go.

import (
	"context"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// autorizadorFalso es el test double de puertos.VerificadorDeAutorizacion
// usado por los demás casos de uso como colaborador de salida (§3.2, §3.3,
// §3.4, §3.5, §3.6 del diseño consumen el caso de uso Autorizar a través de
// esta misma interfaz de entrada). No vive en puertos/mocks porque
// VerificadorDeAutorizacion es un puerto de ENTRADA (lo implementa
// AutorizarCasoDeUso, probado por separado en autorizar_test.go): este
// paquete de test solo necesita sustituirlo por un doble simple. Por
// defecto (FnAutorizar sin configurar) concede el permiso pedido con rol
// "propietario", el camino feliz más común para ejercer los demás casos de
// uso sin tener que reconstruir la matriz de permisos en cada test.
type autorizadorFalso struct {
	FnAutorizar func(ctx context.Context, q puertos.ConsultaAutorizacion) (puertos.Autorizacion, error)

	LlamadasAutorizar []puertos.ConsultaAutorizacion
}

var _ puertos.VerificadorDeAutorizacion = (*autorizadorFalso)(nil)

func (a *autorizadorFalso) Autorizar(ctx context.Context, q puertos.ConsultaAutorizacion) (puertos.Autorizacion, error) {
	a.LlamadasAutorizar = append(a.LlamadasAutorizar, q)
	if a.FnAutorizar != nil {
		return a.FnAutorizar(ctx, q)
	}
	return puertos.Autorizacion{Permitido: true, IDOrganizacion: q.IDOrganizacion, Rol: dominio.RolPropietario.Valor()}, nil
}

// autorizadorDenegado devuelve un autorizadorFalso que siempre deniega con
// el motivo dado (y, si corresponde, el rol actual del sujeto).
func autorizadorDenegado(motivo dominio.MotivoDenegacion, rolActual string) *autorizadorFalso {
	return &autorizadorFalso{
		FnAutorizar: func(ctx context.Context, q puertos.ConsultaAutorizacion) (puertos.Autorizacion, error) {
			return puertos.Autorizacion{IDOrganizacion: q.IDOrganizacion, Rol: rolActual, Motivo: motivo.Valor()}, nil
		},
	}
}

// idUsuarioValido1/2/3 son UUIDs sintácticamente válidos y distintos.
const (
	idUsuarioValido1 = "123e4567-e89b-12d3-a456-426614174000"
	idUsuarioValido2 = "223e4567-e89b-12d3-a456-426614174001"
	idUsuarioValido3 = "323e4567-e89b-12d3-a456-426614174002"
)

// idOrganizacionValido1/2 son UUIDs sintácticamente válidos y distintos.
const (
	idOrganizacionValido1 = "018e7e6a-0000-7000-8000-000000000101"
	idOrganizacionValido2 = "018e7e6a-0000-7000-8000-000000000102"
)

// idMembresiaValido1/2/3 son UUIDs sintácticamente válidos y distintos.
const (
	idMembresiaValido1 = "018e7e6a-0000-7000-8000-000000000201"
	idMembresiaValido2 = "018e7e6a-0000-7000-8000-000000000202"
	idMembresiaValido3 = "018e7e6a-0000-7000-8000-000000000203"
)

// idInvitacionValido1 es un UUID sintácticamente válido.
const idInvitacionValido1 = "018e7e6a-0000-7000-8000-000000000301"

// tokenInvitacionValor1/2 son dos tokens de invitación en claro con forma
// válida (prefijo mot_inv_ + 43 caracteres base64url) y distintos.
const (
	tokenInvitacionValor1 = "mot_inv_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	tokenInvitacionValor2 = "mot_inv_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
)

func ahoraDePrueba() time.Time {
	return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
}

func idUsuarioDePrueba(t *testing.T, uuid string) dominio.IDUsuario {
	t.Helper()
	id, err := dominio.IDUsuarioDesde(uuid)
	if err != nil {
		t.Fatalf("no se pudo construir el IDUsuario de prueba %q: %v", uuid, err)
	}
	return id
}

func idOrganizacionDePrueba(t *testing.T, uuid string) dominio.IDOrganizacion {
	t.Helper()
	id, err := dominio.IDOrganizacionDesde(uuid)
	if err != nil {
		t.Fatalf("no se pudo construir el IDOrganizacion de prueba %q: %v", uuid, err)
	}
	return id
}

func origenDePrueba(t *testing.T) dominio.OrigenSolicitud {
	t.Helper()
	o, err := dominio.NuevoOrigenSolicitud("203.0.113.7", "agente-test/1.0", "huella-test", "req-tenencia-123")
	if err != nil {
		t.Fatalf("no se pudo construir el origen de prueba: %v", err)
	}
	return o
}

func politicaDePrueba(t *testing.T) dominio.PoliticaOrganizacion {
	t.Helper()
	p, err := dominio.NuevaPoliticaOrganizacion(7*24*time.Hour, 200, 50, 20)
	if err != nil {
		t.Fatalf("no se pudo construir la política de organización de prueba: %v", err)
	}
	return p
}

func aliasDePrueba(t *testing.T, valor string) dominio.AliasOrganizacion {
	t.Helper()
	a, err := dominio.NuevoAlias(valor)
	if err != nil {
		t.Fatalf("no se pudo construir el alias de prueba %q: %v", valor, err)
	}
	return a
}

func nombreDePrueba(t *testing.T, valor string) dominio.NombreOrganizacion {
	t.Helper()
	n, err := dominio.NuevoNombre(valor)
	if err != nil {
		t.Fatalf("no se pudo construir el nombre de prueba %q: %v", valor, err)
	}
	return n
}

func correoDePrueba(t *testing.T, valor string) dominio.CorreoDestinatario {
	t.Helper()
	c, err := dominio.NuevoCorreoDestinatario(valor)
	if err != nil {
		t.Fatalf("no se pudo construir el correo de prueba %q: %v", valor, err)
	}
	return c
}

// organizacionActivaDePrueba construye, vía dominio.ReconstituirOrganizacion,
// una Organizacion activa. No acumula eventos.
func organizacionActivaDePrueba(t *testing.T, id, alias, nombre string, creadaPor dominio.IDUsuario, ahora time.Time) *dominio.Organizacion {
	t.Helper()
	return dominio.ReconstituirOrganizacion(
		idOrganizacionDePrueba(t, id),
		aliasDePrueba(t, alias),
		nombreDePrueba(t, nombre),
		dominio.EstadoOrganizacionActiva,
		creadaPor,
		ahora, ahora, nil, dominio.MotivoCambioEstado{},
	)
}

// organizacionSuspendidaDePrueba construye una Organizacion suspendida.
func organizacionSuspendidaDePrueba(t *testing.T, id, alias, nombre string, creadaPor dominio.IDUsuario, ahora time.Time) *dominio.Organizacion {
	t.Helper()
	motivo, err := dominio.NuevoMotivo("investigación de abuso")
	if err != nil {
		t.Fatalf("no se pudo construir el motivo de prueba: %v", err)
	}
	return dominio.ReconstituirOrganizacion(
		idOrganizacionDePrueba(t, id),
		aliasDePrueba(t, alias),
		nombreDePrueba(t, nombre),
		dominio.EstadoOrganizacionSuspendida,
		creadaPor,
		ahora, ahora, nil, motivo,
	)
}

// membresiaActivaDePrueba construye, vía dominio.ReconstituirMembresia, una
// Membresia activa con el rol dado. No acumula eventos.
func membresiaActivaDePrueba(t *testing.T, idMembresia string, idOrganizacion dominio.IDOrganizacion, idUsuario dominio.IDUsuario, rol dominio.Rol, ahora time.Time) *dominio.Membresia {
	t.Helper()
	id, err := dominio.IDMembresiaDesde(idMembresia)
	if err != nil {
		t.Fatalf("no se pudo construir el IDMembresia de prueba %q: %v", idMembresia, err)
	}
	return dominio.ReconstituirMembresia(id, idOrganizacion, idUsuario, rol, dominio.EstadoMembresiaActiva, nil, ahora, ahora, nil)
}

// membresiaSuspendidaDePrueba construye una Membresia suspendida con el rol
// dado.
func membresiaSuspendidaDePrueba(t *testing.T, idMembresia string, idOrganizacion dominio.IDOrganizacion, idUsuario dominio.IDUsuario, rol dominio.Rol, ahora time.Time) *dominio.Membresia {
	t.Helper()
	id, err := dominio.IDMembresiaDesde(idMembresia)
	if err != nil {
		t.Fatalf("no se pudo construir el IDMembresia de prueba %q: %v", idMembresia, err)
	}
	return dominio.ReconstituirMembresia(id, idOrganizacion, idUsuario, rol, dominio.EstadoMembresiaSuspendida, nil, ahora, ahora, nil)
}

// invitacionPendienteDePrueba construye, vía dominio.ReconstituirInvitacion,
// una Invitacion pendiente con el hash del token de prueba dado.
func invitacionPendienteDePrueba(
	t *testing.T,
	idOrganizacion dominio.IDOrganizacion,
	correo dominio.CorreoDestinatario,
	rol dominio.Rol,
	invitadaPor dominio.IDUsuario,
	tokenValor string,
	creadaEn time.Time,
	expiraEn time.Time,
) *dominio.Invitacion {
	t.Helper()
	id, err := dominio.IDInvitacionDesde(idInvitacionValido1)
	if err != nil {
		t.Fatalf("no se pudo construir el IDInvitacion de prueba: %v", err)
	}
	tok, err := dominio.NuevoTokenInvitacionPlano(tokenValor)
	if err != nil {
		t.Fatalf("no se pudo construir el token de invitación de prueba %q: %v", tokenValor, err)
	}
	return dominio.ReconstituirInvitacion(id, idOrganizacion, correo, rol, dominio.EstadoInvitacionPendiente, tok.Hash(), invitadaPor, creadaEn, expiraEn, nil)
}
