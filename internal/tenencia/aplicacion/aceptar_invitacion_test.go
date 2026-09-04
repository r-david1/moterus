package aplicacion_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

func comandoAceptarInvitacionValido(t *testing.T) puertos.ComandoAceptarInvitacion {
	t.Helper()
	return puertos.ComandoAceptarInvitacion{
		TokenPlano: tokenInvitacionValor1,
		IDSujeto:   idUsuarioValido2,
		Origen:     origenDePrueba(t),
	}
}

// mocksAceptarInvitacion arma un mocksInvitaciones con una invitación
// pendiente y vigente lista para aceptar, y VerificadorDeSujetos devolviendo
// el correo coincidente por defecto.
func mocksAceptarInvitacionListos(t *testing.T) (*mocksInvitaciones, *dominio.Invitacion) {
	t.Helper()
	m := nuevosMocksInvitaciones(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	correo := correoDePrueba(t, "invitado@ejemplo.com")
	invitacion := invitacionPendienteDePrueba(t, idOrg, correo, dominio.RolMiembro, idUsuarioDePrueba(t, idUsuarioValido1),
		tokenInvitacionValor1, ahoraDePrueba().Add(-time.Hour), ahoraDePrueba().Add(6*24*time.Hour))
	m.invitaciones.FnBuscarPorHash = func(ctx context.Context, h dominio.HashTokenInvitacion) (*dominio.Invitacion, error) {
		return invitacion, nil
	}
	m.sujetos.FnEsElegible = func(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error) {
		return puertos.SujetoElegible{Existe: true, Activo: true, CorreoNormalizado: "invitado@ejemplo.com"}, nil
	}
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return nil, nil
	}
	return m, invitacion
}

func TestAceptarInvitacionCasoDeUso_FlujoFeliz(t *testing.T) {
	m, invitacion := mocksAceptarInvitacionListos(t)
	caso := m.casoDeUso()

	vista, err := caso.Aceptar(context.Background(), comandoAceptarInvitacionValido(t))
	if err != nil {
		t.Fatalf("Aceptar() devolvió error inesperado: %v", err)
	}
	if vista.IDUsuario != idUsuarioValido2 {
		t.Errorf("IDUsuario = %q, esperado %q", vista.IDUsuario, idUsuarioValido2)
	}
	if vista.Rol != dominio.RolMiembro.Valor() {
		t.Errorf("Rol = %q, esperado %q", vista.Rol, dominio.RolMiembro.Valor())
	}
	if !invitacion.Estado().EsIgual(dominio.EstadoInvitacionAceptada) {
		t.Error("la invitación debía quedar aceptada")
	}
	if len(m.membresias.LlamadasGuardar) != 1 {
		t.Errorf("se esperaba 1 Guardar de membresía, hubo %d", len(m.membresias.LlamadasGuardar))
	}
	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 2 || nombres[0] != "InvitacionResuelta" || nombres[1] != "MiembroAgregado" {
		t.Errorf("eventos auditados = %v, esperado [InvitacionResuelta MiembroAgregado]", nombres)
	}
}

// TestAceptarInvitacionCasoDeUso_Idempotente verifica que si el sujeto ya
// tiene una membresía vigente, la invitación se marca aceptada pero no se
// duplica la membresía ni se audita un alta que no ocurrió.
func TestAceptarInvitacionCasoDeUso_Idempotente(t *testing.T) {
	m, invitacion := mocksAceptarInvitacionListos(t)
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	existente := membresiaActivaDePrueba(t, idMembresiaValido3, idOrg, idUsuarioDePrueba(t, idUsuarioValido2), dominio.RolMiembro, ahoraDePrueba())
	m.membresias.FnBuscarVigente = func(ctx context.Context, u dominio.IDUsuario, o dominio.IDOrganizacion) (*dominio.Membresia, error) {
		return existente, nil
	}
	caso := m.casoDeUso()

	vista, err := caso.Aceptar(context.Background(), comandoAceptarInvitacionValido(t))
	if err != nil {
		t.Fatalf("Aceptar() devolvió error inesperado: %v", err)
	}
	if vista.IDMembresia != existente.ID().String() {
		t.Errorf("IDMembresia = %q, esperado el de la membresía existente %q", vista.IDMembresia, existente.ID().String())
	}
	if !invitacion.Estado().EsIgual(dominio.EstadoInvitacionAceptada) {
		t.Error("la invitación debía consumirse igual")
	}
	if len(m.membresias.LlamadasGuardar) != 0 {
		t.Error("no debía crearse una segunda membresía")
	}
	nombres := m.auditoria.NombresEventos()
	if len(nombres) != 1 || nombres[0] != "InvitacionResuelta" {
		t.Errorf("eventos auditados = %v, esperado solo [InvitacionResuelta] (sin MiembroAgregado)", nombres)
	}
}

// TestAceptarInvitacionCasoDeUso_MismoErrorObservable verifica INV-TEN-24:
// token desconocido, expirada, revocada, ya aceptada y correo distinto dan
// el mismo error observable (mismo texto de mensaje), auditando siempre
// InvitacionResuelta/intento_fallido.
func TestAceptarInvitacionCasoDeUso_MismoErrorObservable(t *testing.T) {
	idOrg := idOrganizacionDePrueba(t, idOrganizacionValido1)
	correo := correoDePrueba(t, "invitado@ejemplo.com")
	otroCorreo := correoDePrueba(t, "otro@ejemplo.com")

	casos := map[string]func(t *testing.T) (*mocksInvitaciones, puertos.ComandoAceptarInvitacion){
		"token_desconocido": func(t *testing.T) (*mocksInvitaciones, puertos.ComandoAceptarInvitacion) {
			m := nuevosMocksInvitaciones(t)
			m.invitaciones.FnBuscarPorHash = func(ctx context.Context, h dominio.HashTokenInvitacion) (*dominio.Invitacion, error) {
				return nil, nil
			}
			return m, comandoAceptarInvitacionValido(t)
		},
		"token_malformado": func(t *testing.T) (*mocksInvitaciones, puertos.ComandoAceptarInvitacion) {
			m := nuevosMocksInvitaciones(t)
			cmd := comandoAceptarInvitacionValido(t)
			cmd.TokenPlano = "no-tiene-el-prefijo-correcto"
			return m, cmd
		},
		"revocada": func(t *testing.T) (*mocksInvitaciones, puertos.ComandoAceptarInvitacion) {
			m := nuevosMocksInvitaciones(t)
			inv := invitacionPendienteDePrueba(t, idOrg, correo, dominio.RolMiembro, idUsuarioDePrueba(t, idUsuarioValido1),
				tokenInvitacionValor1, ahoraDePrueba().Add(-time.Hour), ahoraDePrueba().Add(6*24*time.Hour))
			if err := inv.Revocar(ahoraDePrueba()); err != nil {
				t.Fatalf("no se pudo revocar la invitación de prueba: %v", err)
			}
			m.invitaciones.FnBuscarPorHash = func(ctx context.Context, h dominio.HashTokenInvitacion) (*dominio.Invitacion, error) {
				return inv, nil
			}
			m.sujetos.FnEsElegible = func(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error) {
				return puertos.SujetoElegible{Existe: true, Activo: true, CorreoNormalizado: "invitado@ejemplo.com"}, nil
			}
			return m, comandoAceptarInvitacionValido(t)
		},
		"expirada": func(t *testing.T) (*mocksInvitaciones, puertos.ComandoAceptarInvitacion) {
			m := nuevosMocksInvitaciones(t)
			inv := invitacionPendienteDePrueba(t, idOrg, correo, dominio.RolMiembro, idUsuarioDePrueba(t, idUsuarioValido1),
				tokenInvitacionValor1, ahoraDePrueba().Add(-8*24*time.Hour), ahoraDePrueba().Add(-time.Hour))
			m.invitaciones.FnBuscarPorHash = func(ctx context.Context, h dominio.HashTokenInvitacion) (*dominio.Invitacion, error) {
				return inv, nil
			}
			m.sujetos.FnEsElegible = func(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error) {
				return puertos.SujetoElegible{Existe: true, Activo: true, CorreoNormalizado: "invitado@ejemplo.com"}, nil
			}
			return m, comandoAceptarInvitacionValido(t)
		},
		"correo_distinto": func(t *testing.T) (*mocksInvitaciones, puertos.ComandoAceptarInvitacion) {
			m := nuevosMocksInvitaciones(t)
			inv := invitacionPendienteDePrueba(t, idOrg, correo, dominio.RolMiembro, idUsuarioDePrueba(t, idUsuarioValido1),
				tokenInvitacionValor1, ahoraDePrueba().Add(-time.Hour), ahoraDePrueba().Add(6*24*time.Hour))
			m.invitaciones.FnBuscarPorHash = func(ctx context.Context, h dominio.HashTokenInvitacion) (*dominio.Invitacion, error) {
				return inv, nil
			}
			m.sujetos.FnEsElegible = func(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error) {
				return puertos.SujetoElegible{Existe: true, Activo: true, CorreoNormalizado: otroCorreo.Normalizado()}, nil
			}
			return m, comandoAceptarInvitacionValido(t)
		},
	}

	var mensajeReferencia string
	for nombre, preparar := range casos {
		t.Run(nombre, func(t *testing.T) {
			m, cmd := preparar(t)
			caso := m.casoDeUso()

			_, err := caso.Aceptar(context.Background(), cmd)
			if err == nil {
				t.Fatal("se esperaba un error")
			}
			if mensajeReferencia == "" {
				mensajeReferencia = err.Error()
			} else if err.Error() != mensajeReferencia {
				t.Errorf("mensaje de error = %q, esperado el mismo mensaje observable %q (INV-TEN-24)", err.Error(), mensajeReferencia)
			}
			if len(m.auditoria.LlamadasRegistrar) != 1 {
				t.Fatalf("se esperaba 1 auditoría, hubo %d", len(m.auditoria.LlamadasRegistrar))
			}
			evento, ok := m.auditoria.LlamadasRegistrar[0].Evento.(dominio.InvitacionResuelta)
			if !ok {
				t.Fatalf("evento auditado = %T, esperado dominio.InvitacionResuelta", m.auditoria.LlamadasRegistrar[0].Evento)
			}
			if evento.Desenlace != dominio.DesenlaceIntentoFallido || evento.Resultado != dominio.ResultadoFallo {
				t.Errorf("Desenlace/Resultado = %q/%q, esperado intento_fallido/fallo", evento.Desenlace, evento.Resultado)
			}
			if len(m.membresias.LlamadasGuardar) != 0 {
				t.Error("un intento fallido no debía crear ninguna membresía")
			}
		})
	}
}

func TestAceptarInvitacionCasoDeUso_SujetoNoElegible(t *testing.T) {
	m, _ := mocksAceptarInvitacionListos(t)
	m.sujetos.FnEsElegible = func(ctx context.Context, idUsuario string) (puertos.SujetoElegible, error) {
		return puertos.SujetoElegible{Existe: true, Activo: false}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Aceptar(context.Background(), comandoAceptarInvitacionValido(t))
	var errElegible *dominio.ErrSujetoNoElegible
	if !errors.As(err, &errElegible) {
		t.Fatalf("se esperaba *ErrSujetoNoElegible, obtuvo %T: %v", err, err)
	}
}

func TestAceptarInvitacionCasoDeUso_AccesoDenegadoPorConfianza(t *testing.T) {
	m, _ := mocksAceptarInvitacionListos(t)
	m.confianza.FnEvaluar = func(ctx context.Context, s puertos.SolicitudEvaluacion) (puertos.DecisionConfianza, error) {
		return puertos.DecisionConfianza{Permitido: false, Motivo: "riesgo_alto"}, nil
	}
	caso := m.casoDeUso()

	_, err := caso.Aceptar(context.Background(), comandoAceptarInvitacionValido(t))
	var errConfianza *dominio.ErrAccesoDenegadoPorConfianza
	if !errors.As(err, &errConfianza) {
		t.Fatalf("se esperaba *ErrAccesoDenegadoPorConfianza, obtuvo %T: %v", err, err)
	}
	if len(m.invitaciones.LlamadasBuscarPorHash) != 0 {
		t.Error("no debía consultarse la invitación si Confianza ya denegó")
	}
}

func TestAceptarInvitacionCasoDeUso_OrganizacionNoOperativa(t *testing.T) {
	m, _ := mocksAceptarInvitacionListos(t)
	m.organizaciones.FnBuscarPorID = func(ctx context.Context, id dominio.IDOrganizacion) (*dominio.Organizacion, error) {
		return organizacionSuspendidaDePrueba(t, idOrganizacionValido1, "acme", "Acme", idUsuarioDePrueba(t, idUsuarioValido1), ahoraDePrueba()), nil
	}
	caso := m.casoDeUso()

	_, err := caso.Aceptar(context.Background(), comandoAceptarInvitacionValido(t))
	var errNoOperativa *dominio.ErrOrganizacionNoOperativa
	if !errors.As(err, &errNoOperativa) {
		t.Fatalf("se esperaba *ErrOrganizacionNoOperativa, obtuvo %T: %v", err, err)
	}
}
