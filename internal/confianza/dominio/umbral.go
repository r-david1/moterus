package dominio

import "time"

// Umbral describe un límite de tasa: como máximo Limite intentos dentro de
// Ventana. Es el mismo concepto en los dos niveles que evalúa este
// contexto (IP y cuenta); el nivel lo decide quién construye la clave del
// contador (ver aplicacion.EvaluarTrustSignalCasoDeUso), no este tipo.
type Umbral struct {
	Limite  int
	Ventana time.Duration
}

// LimitesPorAccion agrupa los dos niveles simultáneos de rate limiting que
// pide el encargo de seguridad perimetral: por IP (protege contra un solo
// origen ruidoso) y por cuenta/correo normalizado (protege una cuenta
// concreta de credential stuffing distribuido en muchas IPs, incluso antes
// de saber si la cuenta existe). El límite por tenant (protección de un
// tenant ruidoso contra otros) queda fuera de este tipo a propósito: hoy
// ninguna petición HTTP de Identidad resuelve un tenant_id (Tenencia no
// existe todavía como middleware de resolución), así que no hay una clave
// real que limitar — ver ADR 0018, sección "Alcance no cubierto".
type LimitesPorAccion struct {
	IP     Umbral
	Cuenta Umbral
}

// LimitesPorAccion.Cuenta cubre, según la acción, tres claves distintas de
// segundo nivel — "cuenta" es el nombre genérico del campo, no implica
// necesariamente una cuenta de usuario (ver aplicacion.claveLimite):
// correo normalizado (Identidad), sesión/usuario (Acceso), o
// usuario/organización/IP-como-clave-opaca (Tenencia, §11.3 de su diseño).
//
// PoliticaLimites es la tabla cerrada de umbrales por acción. Los números
// están documentados y justificados en el ADR 0018 (no se pueden ajustar
// sin actualizar ese documento): son deliberadamente más agresivos en
// login (fuerza bruta de contraseñas) y reenvíos (spam de correo) que en
// registro (alta legítima en ráfaga, p. ej. una oficina detrás de un NAT).
type PoliticaLimites map[Accion]LimitesPorAccion

// PoliticaLimitesPorDefecto devuelve los umbrales del ADR 0018, más los dos
// que agrega el contexto Acceso (§11.2 del diseño de Acceso,
// docs/design/acceso-bounded-context.md — no calibrados contra tráfico
// real, igual criterio que los de Identidad):
//   - login:                    IP 5/1min   · cuenta 5/15min
//   - registro:                 IP 10/1min  · cuenta 3/15min
//   - reenvio_verificacion:     IP 5/1min   · cuenta 3/15min
//   - renovacion_sesion:        IP 30/1min  · cuenta("sesion:<id>" o
//     "ip:<ip>", ver acceso/adaptadores/confianza) 10/1min — más
//     permisivo que login porque una renovación ocurre en el camino feliz
//     cada `vidaTokenAcceso` (10 min por defecto) por cada sesión activa
//     de un usuario, y una oficina tras NAT con 20 usuarios activos supera
//     3/min de forma perfectamente legítima.
//   - cierre_masivo_sesiones:   IP 5/1min   · cuenta("usuario:<id>") 3/15min
//   - crear_organizacion:       IP 20/hora  · cuenta("usuario:<id>") 5/hora —
//     operación rara y cara; el techo real de organizaciones por usuario lo
//     pone PoliticaOrganizacion.MaximoOrganizacionesPorUsuario, esto solo
//     acota la tasa (§11.3 del diseño de Tenencia).
//   - invitar_miembro:          IP 5/1min   · cuenta("organizacion:<id>") 20/hora —
//     el endpoint envía correo a terceros: el vector de abuso más caro del
//     contexto, porque el costo lo paga la reputación del dominio del
//     servicio.
//   - aceptar_invitacion:       IP 10/1min  · cuenta("ip:<ip>") 10/1min —
//     oráculo de fuerza bruta sobre tokens de invitación, igual que
//     renovacion_sesion lo es sobre tokens de refresco; ambas dimensiones
//     son en la práctica la misma IP (tenencia/aplicacion.claveCuentaPorIP
//     no conoce el correo del sujeto en este punto del flujo), así que se
//     fija el mismo umbral en las dos.
//   - ingreso_a_sala:           IP 20/min · sin límite por cuenta (§12 de
//     docs/design/colas-virtuales.md) — no hay cuenta que limitar (el
//     ingreso a una sala es pre-autenticación); el ingreso es barato y no
//     hay secreto que adivinar, así que el único objetivo es acotar el
//     farming de tickets (§4, INV-COLA-04), no frenar a una oficina tras
//     NAT. Deliberadamente más laxo que login/registro. Sin esta entrada
//     explícita, Para() aplicaría el default fail-safe de 3/min por IP y
//     rompería salas legítimas.
func PoliticaLimitesPorDefecto() PoliticaLimites {
	return PoliticaLimites{
		AccionLogin: {
			IP:     Umbral{Limite: 5, Ventana: time.Minute},
			Cuenta: Umbral{Limite: 5, Ventana: 15 * time.Minute},
		},
		AccionRegistro: {
			IP:     Umbral{Limite: 10, Ventana: time.Minute},
			Cuenta: Umbral{Limite: 3, Ventana: 15 * time.Minute},
		},
		AccionReenvioVerificacion: {
			IP:     Umbral{Limite: 5, Ventana: time.Minute},
			Cuenta: Umbral{Limite: 3, Ventana: 15 * time.Minute},
		},
		AccionRenovacionSesion: {
			IP:     Umbral{Limite: 30, Ventana: time.Minute},
			Cuenta: Umbral{Limite: 10, Ventana: time.Minute},
		},
		AccionCierreMasivoSesiones: {
			IP:     Umbral{Limite: 5, Ventana: time.Minute},
			Cuenta: Umbral{Limite: 3, Ventana: 15 * time.Minute},
		},
		AccionCrearOrganizacion: {
			IP:     Umbral{Limite: 20, Ventana: time.Hour},
			Cuenta: Umbral{Limite: 5, Ventana: time.Hour},
		},
		AccionInvitarMiembro: {
			IP:     Umbral{Limite: 5, Ventana: time.Minute},
			Cuenta: Umbral{Limite: 20, Ventana: time.Hour},
		},
		AccionAceptarInvitacion: {
			IP:     Umbral{Limite: 10, Ventana: time.Minute},
			Cuenta: Umbral{Limite: 10, Ventana: time.Minute},
		},
		// verificar_otp: usuario("usuario:<id>") 5/15min · IP 20/15min (§7 de
		// docs/design/otp-mfa.md) — oráculo de fuerza bruta clásico sobre un
		// código de 6 dígitos (10^6 combinaciones) con ventanas de 30s;
		// deliberadamente más agresivo por cuenta que por IP, mismo criterio
		// que login: un atacante dirigido a una cuenta concreta agota su
		// cupo mucho antes que uno distribuido en muchas IPs.
		AccionVerificarOTP: {
			IP:     Umbral{Limite: 20, Ventana: 15 * time.Minute},
			Cuenta: Umbral{Limite: 5, Ventana: 15 * time.Minute},
		},
		// ingreso_a_sala: 20/min por IP, sin límite por cuenta (§12 del
		// diseño colas-virtuales.md: no hay cuenta que limitar, es
		// pre-autenticación). El campo Cuenta nunca se evalúa en la
		// práctica porque aplicacion.PorteroDeSalaCasoDeUso.Ingresar llama a
		// Evaluar con CorreoNormalizado="" (EvaluarTrustSignalCasoDeUso solo
		// aplica el límite por cuenta cuando ese campo no está vacío); se
		// deja igual al de IP por documentación, no por necesidad.
		AccionIngresoASala: {
			IP:     Umbral{Limite: 20, Ventana: time.Minute},
			Cuenta: Umbral{Limite: 20, Ventana: time.Minute},
		},
	}
}

// Para devuelve los límites configurados para accion. Una acción sin
// entrada explícita en la tabla usa un umbral conservador por defecto (3
// por minuto por IP, 3 en 15 minutos por cuenta) en vez de no limitar nada
// — fail-safe: una acción nueva que alguien olvide registrar aquí queda
// protegida igual, aunque con un umbral genérico, no invisible por
// completo.
func (p PoliticaLimites) Para(accion Accion) LimitesPorAccion {
	if limites, ok := p[accion]; ok {
		return limites
	}
	return LimitesPorAccion{
		IP:     Umbral{Limite: 3, Ventana: time.Minute},
		Cuenta: Umbral{Limite: 3, Ventana: 15 * time.Minute},
	}
}
