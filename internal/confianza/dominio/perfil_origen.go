package dominio

// PerfilDeOrigen es la entidad efímera que representa lo que Confianza sabe
// de los orígenes históricos de una cuenta (§1.2 del diseño): cuántos
// logins exitosos acumuló, cuántos de ellos traían huella, y si el
// dispositivo/red de la petición actual ya estaban entre los conocidos. Su
// estado vive en Redis (adaptadores/redis/perfil_origenes.go, fase
// posterior), nunca en Postgres — no tiene ciclo de vida que modelar más
// allá de esto: se reconstituye entero en cada evaluación a partir de la
// VistaPerfilOrigen que trae el puerto de salida, se usa una vez, se
// descarta.
type PerfilDeOrigen struct {
	clave               ClaveCuenta
	exitos              int64
	exitosConHuella     int64
	dispositivoConocido bool
	redConocida         bool
}

// NuevoPerfilDeOrigen reconstituye un PerfilDeOrigen a partir de campos ya
// resueltos por el adaptador de salida (típicamente el resultado de un
// HMGET sobre confianza:orig:<clave>, §2.2 y §5.2 del diseño): cuántos
// éxitos acumulados, cuántos de ellos con huella, y si el hash de
// dispositivo/red de la petición actual ya figuraban en el perfil. No
// valida relaciones entre exitos y exitosConHuella (exitosConHuella ≤
// exitos es una invariante del adaptador que escribe el HASH, no algo que
// el dominio pueda ni deba re-verificar sin conocer Redis).
func NuevoPerfilDeOrigen(clave ClaveCuenta, exitos, exitosConHuella int64, dispositivoConocido, redConocida bool) PerfilDeOrigen {
	return PerfilDeOrigen{
		clave:               clave,
		exitos:              exitos,
		exitosConHuella:     exitosConHuella,
		dispositivoConocido: dispositivoConocido,
		redConocida:         redConocida,
	}
}

// Clave devuelve la ClaveCuenta de este perfil.
func (p PerfilDeOrigen) Clave() ClaveCuenta { return p.clave }

// Exitos devuelve el número de logins exitosos acumulados en el perfil.
func (p PerfilDeOrigen) Exitos() int64 { return p.exitos }

// ExitosConHuella devuelve cuántos de esos logins exitosos traían huella de
// dispositivo.
func (p PerfilDeOrigen) ExitosConHuella() int64 { return p.exitosConHuella }

// TieneHistorialSuficiente indica si la cuenta acumuló al menos
// politica.MinimoExitosParaJuzgar() logins exitosos (INV-RIES-04). Una
// cuenta por debajo de ese umbral no tiene con qué compararse: penalizar el
// primer login de una cuenta nueva sería penalizar a todo usuario nuevo del
// producto, el falso positivo más caro que existe.
func (p PerfilDeOrigen) TieneHistorialSuficiente(politica PoliticaRiesgo) bool {
	return p.exitos >= politica.MinimoExitosParaJuzgar()
}

// Senales implementa, sin ninguna desviación, el pseudocódigo de seis
// líneas de §1.4 del diseño: la función pura que es todo el "análisis" de
// este mecanismo.
//
//	Senales(obs, politica):
//	    si NO TieneHistorialSuficiente(politica):        → []            (INV-RIES-04)
//	    señales := []
//	    si obs.traeHuella:
//	        si NO dispositivoConocido:   señales += dispositivo_desconocido
//	    si NO obs.traeHuella Y exitosConHuella ≥ politica.MinimoExitosParaJuzgar:
//	                                     señales += huella_ausente
//	    si obs.red != cero Y NO redConocida:
//	                                     señales += red_desconocida
//	    return señales
//
// Tres decisiones no obvias, documentadas en detalle en §1.4 del diseño:
//
//  1. Sin historial suficiente, cero señales (INV-RIES-04).
//  2. huella_ausente mira exitosConHuella, no exitos: si el frontend nunca
//     mandó la cabecera, la ausencia no significa nada; si la mandó varias
//     veces y ahora no, alguien usa un cliente distinto del habitual. Es lo
//     que impide que "no mandes el header" sea una evasión gratis
//     (INV-RIES-06).
//  3. dispositivo_desconocido no se emite si la petición no trae huella:
//     sería contar dos veces la misma ausencia, y las señales tienen que
//     ser independientes o los pesos de PoliticaRiesgo mienten.
func (p PerfilDeOrigen) Senales(obs HuellaDeOrigen, politica PoliticaRiesgo) []SenalRiesgo {
	if !p.TieneHistorialSuficiente(politica) {
		return nil
	}

	var senales []SenalRiesgo

	if obs.TraeHuella() {
		if !p.dispositivoConocido {
			senales = append(senales, SenalDispositivoDesconocido)
		}
	} else if p.exitosConHuella >= politica.MinimoExitosParaJuzgar() {
		senales = append(senales, SenalHuellaAusente)
	}

	if !obs.Red().EsVacio() && !p.redConocida {
		senales = append(senales, SenalRedDesconocida)
	}

	return senales
}
