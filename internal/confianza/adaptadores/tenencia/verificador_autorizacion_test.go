package tenencia

import (
	"context"
	"errors"
	"testing"

	confianzapuertos "github.com/r-david1/moterus/internal/confianza/puertos"
	tenenciapuertos "github.com/r-david1/moterus/internal/tenencia/puertos"
)

// TestPermisosCoincidenConElCatalogoDeTenencia fija la dependencia dura
// entre las constantes locales PermisoOrganizacionEditar/
// PermisoOrganizacionVer y el catálogo cerrado real de Tenencia: si ese
// catálogo cambiara esos valores, este test lo detecta en vez de dejar que
// VerificadorAutorizacion empiece a pedir un permiso inexistente en
// silencio.
func TestPermisosCoincidenConElCatalogoDeTenencia(t *testing.T) {
	if !permisosCoincidenConElCatalogoDeTenencia() {
		t.Fatal("las constantes locales de permiso se desincronizaron del catálogo cerrado de tenencia/dominio")
	}
}

type autorizadorFalso struct {
	fn func(ctx context.Context, q tenenciapuertos.ConsultaAutorizacion) (tenenciapuertos.Autorizacion, error)
}

func (a *autorizadorFalso) Autorizar(ctx context.Context, q tenenciapuertos.ConsultaAutorizacion) (tenenciapuertos.Autorizacion, error) {
	return a.fn(ctx, q)
}

// TestVerificadorAutorizacion_Autorizar_TraduceYColapsaLaRespuesta cubre el
// contrato central del ACL: traduce ConsultaAutorizacionOrganizacion (de
// Confianza) a ConsultaAutorizacion (de Tenencia) preservando IDSujeto,
// IDOrganizacion y Permiso, y colapsa la Autorizacion rica de Tenencia
// (Permitido + Motivo) al booleano estrecho que exige el puerto de
// Confianza — sin importar cuál sea el Motivo de una denegación.
func TestVerificadorAutorizacion_Autorizar_TraduceYColapsaLaRespuesta(t *testing.T) {
	casos := []struct {
		nombre    string
		respuesta tenenciapuertos.Autorizacion
		esperado  bool
	}{
		{"permitido sin motivo", tenenciapuertos.Autorizacion{Permitido: true}, true},
		{"denegado por rol insuficiente se colapsa a false", tenenciapuertos.Autorizacion{Permitido: false, Motivo: "rol_insuficiente"}, false},
		{"denegado por falta de membresía se colapsa a false, igual que rol insuficiente", tenenciapuertos.Autorizacion{Permitido: false, Motivo: "sin_membresia"}, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var capturada tenenciapuertos.ConsultaAutorizacion
			acl := NuevoVerificadorAutorizacion(&autorizadorFalso{
				fn: func(ctx context.Context, q tenenciapuertos.ConsultaAutorizacion) (tenenciapuertos.Autorizacion, error) {
					capturada = q
					return c.respuesta, nil
				},
			})

			permitido, err := acl.Autorizar(context.Background(), confianzapuertos.ConsultaAutorizacionOrganizacion{
				IDSujeto:       "018e7e6a-0000-7000-8000-000000000001",
				IDOrganizacion: "018e7e6a-0000-7000-8000-000000000002",
				Permiso:        PermisoOrganizacionEditar,
			})
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if permitido != c.esperado {
				t.Fatalf("permitido = %v, esperado %v", permitido, c.esperado)
			}
			if capturada.IDUsuario != "018e7e6a-0000-7000-8000-000000000001" {
				t.Fatalf("IDUsuario no se propagó correctamente: %q", capturada.IDUsuario)
			}
			if capturada.IDOrganizacion != "018e7e6a-0000-7000-8000-000000000002" {
				t.Fatalf("IDOrganizacion no se propagó correctamente: %q", capturada.IDOrganizacion)
			}
			if capturada.Permiso != PermisoOrganizacionEditar {
				t.Fatalf("Permiso no se propagó correctamente: %q", capturada.Permiso)
			}
		})
	}
}

// TestVerificadorAutorizacion_Autorizar_PropagaError cubre que un error de
// infraestructura o de Tenencia se propague tal cual (comentario del puerto
// de Confianza: "el llamador debe tratarlo como 'no se pudo autorizar',
// nunca como 'sí autorizado' por defecto").
func TestVerificadorAutorizacion_Autorizar_PropagaError(t *testing.T) {
	esperado := errors.New("fallo de infraestructura")
	acl := NuevoVerificadorAutorizacion(&autorizadorFalso{
		fn: func(ctx context.Context, q tenenciapuertos.ConsultaAutorizacion) (tenenciapuertos.Autorizacion, error) {
			return tenenciapuertos.Autorizacion{Permitido: true}, esperado
		},
	})

	permitido, err := acl.Autorizar(context.Background(), confianzapuertos.ConsultaAutorizacionOrganizacion{
		IDSujeto:       "018e7e6a-0000-7000-8000-000000000001",
		IDOrganizacion: "018e7e6a-0000-7000-8000-000000000002",
		Permiso:        PermisoOrganizacionVer,
	})
	if !errors.Is(err, esperado) {
		t.Fatalf("error = %v, esperado %v", err, esperado)
	}
	if permitido {
		t.Fatal("permitido debe ser false cuando Autorizar devuelve un error, nunca true por defecto")
	}
}
