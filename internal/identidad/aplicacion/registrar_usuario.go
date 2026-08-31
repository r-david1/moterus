package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/identidad/dominio"
	"github.com/r-david1/moterus/internal/identidad/puertos"
)

// RegistrarUsuarioCasoDeUso implementa puertos.RegistradorDeUsuarios: el
// alta de un nuevo Usuario en pendiente_verificacion (sección 3.1 del
// diseño). Es una única transacción de negocio: registrar un usuario y
// nada más.
type RegistrarUsuarioCasoDeUso struct {
	usuarios           puertos.RepositorioUsuarios
	hasher             puertos.HasherContrasenas
	filtradas          puertos.VerificadorContrasenasFiltradas
	confianza          puertos.EvaluadorConfianza
	auditoria          puertos.RegistroAuditoria
	eventos            puertos.PublicadorEventos
	reloj              puertos.Reloj
	ids                puertos.GeneradorIDs
	uow                puertos.UnidadDeTrabajo
	generadorTokens    puertos.GeneradorTokens
	tokensVerificacion puertos.RepositorioTokensVerificacion
	notificadorCorreo  puertos.NotificadorCorreo
	politica           dominio.PoliticaContrasena
}

var _ puertos.RegistradorDeUsuarios = (*RegistrarUsuarioCasoDeUso)(nil)

// NuevoRegistrarUsuarioCasoDeUso construye el caso de uso con sus
// dependencias inyectadas por puerto (nunca implementaciones concretas).
func NuevoRegistrarUsuarioCasoDeUso(
	usuarios puertos.RepositorioUsuarios,
	hasher puertos.HasherContrasenas,
	filtradas puertos.VerificadorContrasenasFiltradas,
	confianza puertos.EvaluadorConfianza,
	auditoria puertos.RegistroAuditoria,
	eventos puertos.PublicadorEventos,
	reloj puertos.Reloj,
	ids puertos.GeneradorIDs,
	uow puertos.UnidadDeTrabajo,
	generadorTokens puertos.GeneradorTokens,
	tokensVerificacion puertos.RepositorioTokensVerificacion,
	notificadorCorreo puertos.NotificadorCorreo,
) *RegistrarUsuarioCasoDeUso {
	return &RegistrarUsuarioCasoDeUso{
		usuarios:           usuarios,
		hasher:             hasher,
		filtradas:          filtradas,
		confianza:          confianza,
		auditoria:          auditoria,
		eventos:            eventos,
		reloj:              reloj,
		ids:                ids,
		uow:                uow,
		generadorTokens:    generadorTokens,
		tokensVerificacion: tokensVerificacion,
		notificadorCorreo:  notificadorCorreo,
		politica:           dominio.NuevaPoliticaContrasena(),
	}
}

// Registrar ejecuta el flujo normativo de la sección 3.1 del diseño, en
// este orden: evaluar confianza -> construir VOs -> política de
// contraseña -> verificador de filtradas (fail-open) -> hashear -> crear
// agregado -> persistir+auditar en UnidadDeTrabajo -> publicar eventos ->
// registrar resultado en Confianza.
func (c *RegistrarUsuarioCasoDeUso) Registrar(ctx context.Context, cmd puertos.ComandoRegistrarUsuario) (puertos.ResultadoRegistro, error) {
	// 1. Confianza se consulta antes que cualquier otra cosa (misma
	// disciplina que en el login, aunque aquí no hay INV-ID-12 dedicada:
	// evita que un abuso de registro llegue siquiera a construir VOs).
	decision, err := c.confianza.Evaluar(ctx, puertos.SolicitudEvaluacion{
		Accion:            "registro",
		CorreoNormalizado: cmd.Correo,
		Origen:            cmd.Origen,
		TokenCaptcha:      cmd.TokenCaptcha,
	})
	if err != nil {
		return puertos.ResultadoRegistro{}, err
	}
	if !decision.Permitido {
		evento := dominio.NuevoRegistroRechazado(cmd.Correo, decision.Motivo, c.reloj.Ahora())
		if errAud := c.auditoria.Registrar(ctx, evento, cmd.Origen); errAud != nil {
			// INV-ID-15: si no se puede probar lo que pasó, no pasa. Se
			// prioriza el error de auditoría sobre el de negocio.
			return puertos.ResultadoRegistro{}, errAud
		}
		return puertos.ResultadoRegistro{}, &dominio.ErrAccesoDenegadoPorConfianza{Motivo: decision.Motivo, ReintentarEn: decision.ReintentarEn}
	}

	// 2. Construcción de VOs: todo input inválido produce un error de
	// dominio uniforme aquí, no en el adaptador (ADR candidato 0016).
	correo, err := dominio.NuevoCorreo(cmd.Correo)
	if err != nil {
		return puertos.ResultadoRegistro{}, err
	}
	plana, err := dominio.NuevaContrasenaPlana(cmd.Contrasena)
	if err != nil {
		return puertos.ResultadoRegistro{}, err
	}

	// 3. Fortaleza de la contraseña (NIST SP 800-63B, sin red).
	if err := c.politica.Evaluar(plana, correo); err != nil {
		return puertos.ResultadoRegistro{}, err
	}

	// 4. Brechas conocidas (HIBP): fail-open deliberado. Una caída del
	// puerto no puede bloquear todas las altas.
	filtrada, err := c.filtradas.EstaFiltrada(ctx, plana)
	if err != nil {
		slog.WarnContext(ctx, "verificador de contraseñas filtradas falló; continuando fail-open",
			"error", err, "correo_dominio", correo.Dominio())
	} else if filtrada {
		return puertos.ResultadoRegistro{}, &dominio.ErrContrasenaFiltrada{}
	}

	// 5. Hashing.
	hash, err := c.hasher.Hashear(ctx, plana)
	if err != nil {
		return puertos.ResultadoRegistro{}, err
	}

	// 6. Crear el agregado en pendiente_verificacion (INV-ID-07); acumula
	// UsuarioRegistrado.
	id, err := c.ids.NuevoIDUsuario()
	if err != nil {
		return puertos.ResultadoRegistro{}, err
	}
	ahora := c.reloj.Ahora()
	usuario, err := dominio.RegistrarUsuario(id, correo, hash, ahora)
	if err != nil {
		return puertos.ResultadoRegistro{}, err
	}
	eventosPendientes := usuario.EventosPendientes()

	// 7. Generar el token de verificación de correo (sección 3.4 del
	// diseño): un secreto de un solo uso, de alta entropía, independiente
	// de la contraseña, que demuestra acceso al buzón. Si el generador
	// falla, se aborta todo el registro: sin token no hay forma de que el
	// usuario recién creado pase de pendiente_verificacion a activo.
	tokenPlano, err := c.generadorTokens.Generar()
	if err != nil {
		return puertos.ResultadoRegistro{}, err
	}
	hashToken := hashTokenVerificacion(tokenPlano)
	expiraToken := ahora.Add(vigenciaTokenVerificacionCorreo)

	// 8. Persistir el usuario, el hash del token de verificación y auditar,
	// todo en la misma transacción (ADR 0005). El token en claro nunca
	// llega a este paso: solo su hash (INV-ID-21).
	err = c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		if err := c.usuarios.Guardar(ctx, usuario); err != nil {
			return err
		}
		if err := c.tokensVerificacion.Guardar(ctx, id, hashToken, expiraToken); err != nil {
			return err
		}
		for _, e := range eventosPendientes {
			if err := c.auditoria.Registrar(ctx, e, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return puertos.ResultadoRegistro{}, err
	}

	// 9. Envío del correo de verificación: best-effort, fuera de la
	// transacción, igual criterio que la publicación de eventos. Es el
	// único lugar donde el token viaja en claro (INV-ID-21): nunca se
	// loguea y nunca llega a ResultadoRegistro/la respuesta HTTP.
	if errNotif := c.notificadorCorreo.EnviarVerificacion(ctx, correo, tokenPlano); errNotif != nil {
		slog.WarnContext(ctx, "envío del correo de verificación falló",
			"error", errNotif, "usuario_id", id.String())
	}

	// 10. Publicación asíncrona fuera de la transacción: best-effort, no
	// aborta el registro si falla.
	if errPub := c.eventos.Publicar(ctx, eventosPendientes...); errPub != nil {
		slog.WarnContext(ctx, "publicación de eventos de registro falló",
			"error", errPub, "usuario_id", id.String())
	}

	// 11. Confianza registra el resultado del intento.
	if errConf := c.confianza.RegistrarResultado(ctx, puertos.ResultadoIntento{
		Accion:            "registro",
		CorreoNormalizado: correo.Normalizado(),
		Origen:            cmd.Origen,
		Exitoso:           true,
		UsuarioID:         id.String(),
	}); errConf != nil {
		slog.WarnContext(ctx, "registro de resultado en Confianza falló",
			"error", errConf, "usuario_id", id.String())
	}

	return puertos.ResultadoRegistro{
		IDUsuario:                  id.String(),
		Estado:                     usuario.Estado().String(),
		RequiereVerificacionCorreo: true,
	}, nil
}
