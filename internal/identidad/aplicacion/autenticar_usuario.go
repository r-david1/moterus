package aplicacion

import (
	"context"
	"errors"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// AutenticarUsuarioCasoDeUso implementa puertos.AutenticadorDeCredenciales:
// verifica credenciales, evalúa Confianza y dispara el gancho de segundo
// factor si aplica (sección 3.2 del diseño). Nunca emite tokens ni sesión
// (INV-ID-14, ADR candidato 0009): eso es responsabilidad del caso de uso
// IniciarSesion del contexto Acceso, que orquesta este caso de uso.
type AutenticarUsuarioCasoDeUso struct {
	usuarios  puertos.RepositorioUsuarios
	hasher    puertos.HasherContrasenas
	confianza puertos.EvaluadorConfianza
	auditoria puertos.RegistroAuditoria
	eventos   puertos.PublicadorEventos
	reloj     puertos.Reloj
	uow       puertos.UnidadDeTrabajo
}

var _ puertos.AutenticadorDeCredenciales = (*AutenticarUsuarioCasoDeUso)(nil)

// NuevoAutenticarUsuarioCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto.
func NuevoAutenticarUsuarioCasoDeUso(
	usuarios puertos.RepositorioUsuarios,
	hasher puertos.HasherContrasenas,
	confianza puertos.EvaluadorConfianza,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	uow puertos.UnidadDeTrabajo,
) *AutenticarUsuarioCasoDeUso {
	return &AutenticarUsuarioCasoDeUso{
		usuarios:  usuarios,
		hasher:    hasher,
		confianza: confianza,
		auditoria: auditoria,
		eventos:   eventos,
		reloj:     reloj,
		uow:       uow,
	}
}

// Autenticar ejecuta el flujo normativo de la sección 3.2 del diseño. El
// orden es normativo, no cosmético (INV-ID-11, INV-ID-12, INV-ID-13):
//  1. Evaluar Confianza ANTES de tocar la base de datos.
//  2. Construir Correo; si es inválido, mismo error genérico que "no existe".
//  3. Buscar por correo; si no existe, tiempo equivalente + error genérico.
//  4. Verificar contraseña; si falla, error genérico (ya con usuario_id).
//  5. Solo con la contraseña ya verificada, comprobar que el usuario puede
//     iniciar sesión (estado del ciclo de vida).
//  6. Rehash oportunista no bloqueante.
//  7. Registrar acceso y persistir junto con la auditoría de éxito.
//  8. Determinar si se requiere segundo factor.
//  9. Registrar el resultado en Confianza.
func (c *AutenticarUsuarioCasoDeUso) Autenticar(ctx context.Context, cmd puertos.ComandoAutenticar) (puertos.ResultadoAutenticacion, error) {
	ahora := c.reloj.Ahora()

	// 1. INV-ID-12: ningún intento llega al repositorio sin pasar antes por
	// Confianza. Evita que el credential stuffing consuma conexiones a
	// Postgres.
	decision, err := c.confianza.Evaluar(ctx, puertos.SolicitudEvaluacion{
		Accion:            "login",
		CorreoNormalizado: cmd.Correo,
		Origen:            cmd.Origen,
		TokenCaptcha:      cmd.TokenCaptcha,
	})
	if err != nil {
		return puertos.ResultadoAutenticacion{}, err
	}
	if !decision.Permitido {
		evento := dominio.NuevoAutenticacionDenegada(cmd.Correo, decision.Motivo, ahora)
		if errAud := c.auditoria.Registrar(ctx, evento, cmd.Origen); errAud != nil {
			return puertos.ResultadoAutenticacion{}, errAud
		}
		return puertos.ResultadoAutenticacion{}, &dominio.ErrAccesoDenegadoPorConfianza{Motivo: decision.Motivo}
	}

	// 2. Un correo malformado no debe distinguirse de uno inexistente
	// (INV-ID-11): mismo error, mismo tiempo de respuesta equivalente.
	correo, err := dominio.NuevoCorreo(cmd.Correo)
	if err != nil {
		c.hasher.ConsumirTiempoEquivalente(ctx)
		if errAud := c.registrarFalloLogin(ctx, cmd.Correo, "", "correo_invalido", cmd.Origen); errAud != nil {
			return puertos.ResultadoAutenticacion{}, errAud
		}
		return puertos.ResultadoAutenticacion{}, &dominio.ErrCredencialesInvalidas{}
	}

	// 3. Correo inexistente: mismo error y tiempo equivalente. No se
	// invoca Verificar, por lo que hace falta el señuelo criptográfico.
	usuario, err := c.usuarios.BuscarPorCorreo(ctx, correo)
	if err != nil {
		var noEncontrado *dominio.ErrUsuarioNoEncontrado
		if !errors.As(err, &noEncontrado) {
			return puertos.ResultadoAutenticacion{}, err
		}
		usuario = nil
	}
	if usuario == nil {
		c.hasher.ConsumirTiempoEquivalente(ctx)
		if errAud := c.registrarFalloLogin(ctx, correo.Normalizado(), "", "usuario_no_encontrado", cmd.Origen); errAud != nil {
			return puertos.ResultadoAutenticacion{}, errAud
		}
		return puertos.ResultadoAutenticacion{}, &dominio.ErrCredencialesInvalidas{}
	}

	// 4. Contraseña incorrecta: mismo error genérico, pero ya con
	// usuario_id conocido para la auditoría. No hace falta tiempo
	// equivalente adicional: Verificar (o el rechazo estructural de
	// ContrasenaPlana) ya consumió un coste comparable.
	plana, err := dominio.NuevaContrasenaPlana(cmd.Contrasena)
	if err != nil {
		if errAud := c.registrarFalloLogin(ctx, correo.Normalizado(), usuario.ID().String(), "contrasena_invalida", cmd.Origen); errAud != nil {
			return puertos.ResultadoAutenticacion{}, errAud
		}
		return puertos.ResultadoAutenticacion{}, &dominio.ErrCredencialesInvalidas{}
	}
	verificada, err := c.hasher.Verificar(ctx, usuario.Credencial().Hash(), plana)
	if err != nil {
		return puertos.ResultadoAutenticacion{}, err
	}
	if !verificada {
		if errAud := c.registrarFalloLogin(ctx, correo.Normalizado(), usuario.ID().String(), "contrasena_incorrecta", cmd.Origen); errAud != nil {
			return puertos.ResultadoAutenticacion{}, errAud
		}
		return puertos.ResultadoAutenticacion{}, &dominio.ErrCredencialesInvalidas{}
	}

	// 5. Solo tras verificar la contraseña se revela el estado de la
	// cuenta (INV-ID-06): un atacante que no la conoce no debe distinguir
	// una cuenta suspendida de una inexistente. Se devuelve el error de
	// dominio real (ErrCorreoNoVerificado/ErrCuentaSuspendida/
	// ErrCuentaBloqueada), no el genérico.
	if errEstado := usuario.PuedeIniciarSesion(); errEstado != nil {
		motivo := motivoLoginDenegado(errEstado)
		if errAud := c.registrarFalloLogin(ctx, correo.Normalizado(), usuario.ID().String(), motivo, cmd.Origen); errAud != nil {
			return puertos.ResultadoAutenticacion{}, errAud
		}
		return puertos.ResultadoAutenticacion{}, errEstado
	}

	// 6. Rehash oportunista (INV-ID-13): su fallo nunca aborta el login.
	if c.hasher.NecesitaRehash(usuario.Credencial().Hash()) {
		nuevoHash, errHash := c.hasher.Hashear(ctx, plana)
		if errHash != nil {
			slog.WarnContext(ctx, "rehash oportunista falló al hashear; se continúa el login",
				"error", errHash, "usuario_id", usuario.ID().String())
		} else if errReemplazo := usuario.ReemplazarHash(nuevoHash, ahora); errReemplazo != nil {
			slog.WarnContext(ctx, "rehash oportunista falló al reemplazar el hash; se continúa el login",
				"error", errReemplazo, "usuario_id", usuario.ID().String())
		}
	}

	// 7. Registrar el acceso.
	usuario.RegistrarAcceso(ahora)

	// 8. Segundo factor: por MFA propio del usuario o por exigencia de
	// Confianza.
	requiereSegundoFactor := usuario.TieneMFA() || decision.RequiereStepUp
	motivoStepUp := ""
	switch {
	case usuario.TieneMFA():
		motivoStepUp = "mfa_habilitado"
	case decision.RequiereStepUp:
		motivoStepUp = "confianza_baja"
	}

	eventosDominio := usuario.EventosPendientes() // p. ej. CredencialRehasheada
	eventoExitoso := dominio.NuevoAutenticacionExitosa(usuario.ID(), usuario.Correo(), ahora)
	eventosAuditar := append([]dominio.EventoDominio{eventoExitoso}, eventosDominio...)
	if requiereSegundoFactor {
		eventosAuditar = append(eventosAuditar, dominio.NuevoSegundoFactorRequerido(usuario.ID(), motivoStepUp, ahora))
	}

	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.usuarios.Guardar(ctx, usuario); err != nil {
			return err
		}
		for _, e := range eventosAuditar {
			if err := c.auditoria.Registrar(ctx, e, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return puertos.ResultadoAutenticacion{}, err
	}

	// Publicación asíncrona fuera de la transacción: best-effort.
	if errPub := c.eventos.Publicar(ctx, eventosAuditar...); errPub != nil {
		slog.WarnContext(ctx, "publicación de eventos de login falló",
			"error", errPub, "usuario_id", usuario.ID().String())
	}

	// 9. Confianza registra el resultado del intento exitoso.
	if errConf := c.confianza.RegistrarResultado(ctx, puertos.ResultadoIntento{
		Accion:            "login",
		CorreoNormalizado: usuario.Correo().Normalizado(),
		Origen:            cmd.Origen,
		Exitoso:           true,
		UsuarioID:         usuario.ID().String(),
	}); errConf != nil {
		slog.WarnContext(ctx, "registro de resultado en Confianza falló",
			"error", errConf, "usuario_id", usuario.ID().String())
	}

	return puertos.ResultadoAutenticacion{
		IDUsuario:             usuario.ID().String(),
		CorreoNormalizado:     usuario.Correo().Normalizado(),
		Estado:                usuario.Estado().String(),
		RequiereSegundoFactor: requiereSegundoFactor,
		MotivoStepUp:          motivoStepUp,
		PuntajeConfianza:      decision.Puntaje,
	}, nil
}

// registrarFalloLogin agrupa el efecto secundario común a todo intento de
// login fallido: auditar AutenticacionFallida (INV-ID-15: acción de
// auditoría del catálogo cerrado, con accion "usuario.login" y resultado
// "fallo") y avisar a Confianza. Solo devuelve error cuando falla el
// propio registro de auditoría: en ese caso, según INV-ID-15, ese error se
// propaga en lugar del error de negocio (si no se puede probar lo que
// pasó, no pasa). El fallo al avisar a Confianza es best-effort.
func (c *AutenticarUsuarioCasoDeUso) registrarFalloLogin(ctx context.Context, correoNormalizado, idUsuario, motivo string, origen dominio.OrigenSolicitud) error {
	ahora := c.reloj.Ahora()
	evento := dominio.NuevoAutenticacionFallida(idUsuario, correoNormalizado, motivo, ahora)
	if err := c.auditoria.Registrar(ctx, evento, origen); err != nil {
		return err
	}
	if err := c.confianza.RegistrarResultado(ctx, puertos.ResultadoIntento{
		Accion:            "login",
		CorreoNormalizado: correoNormalizado,
		Origen:            origen,
		Exitoso:           false,
		UsuarioID:         idUsuario,
	}); err != nil {
		slog.WarnContext(ctx, "registro de resultado en Confianza falló", "error", err)
	}
	return nil
}

// motivoLoginDenegado traduce un error de PuedeIniciarSesion al motivo de
// auditoría del catálogo cerrado (INV-ID-17: sin concatenar strings ad hoc
// en el caso de uso).
func motivoLoginDenegado(err error) string {
	var noVerificado *dominio.ErrCorreoNoVerificado
	var suspendida *dominio.ErrCuentaSuspendida
	var bloqueada *dominio.ErrCuentaBloqueada
	switch {
	case errors.As(err, &noVerificado):
		return "correo_no_verificado"
	case errors.As(err, &suspendida):
		return "cuenta_suspendida"
	case errors.As(err, &bloqueada):
		return "cuenta_bloqueada"
	default:
		return "estado_no_operativo"
	}
}
