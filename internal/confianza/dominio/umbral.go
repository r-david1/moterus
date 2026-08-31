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

// PoliticaLimites es la tabla cerrada de umbrales por acción. Los números
// están documentados y justificados en el ADR 0018 (no se pueden ajustar
// sin actualizar ese documento): son deliberadamente más agresivos en
// login (fuerza bruta de contraseñas) y reenvíos (spam de correo) que en
// registro (alta legítima en ráfaga, p. ej. una oficina detrás de un NAT).
type PoliticaLimites map[Accion]LimitesPorAccion

// PoliticaLimitesPorDefecto devuelve los umbrales del ADR 0018:
//   - login:                IP 5/1min   · cuenta 5/15min
//   - registro:             IP 10/1min  · cuenta 3/15min
//   - reenvio_verificacion: IP 5/1min   · cuenta 3/15min
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
