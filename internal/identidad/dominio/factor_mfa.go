package dominio

import "time"

// FactorMFA es el agregado raíz del contexto Identidad que representa un
// segundo factor de autenticación de un Usuario (§1.2-1.3 del diseño
// otp-mfa.md). Es un agregado propio, referenciado por IDUsuario y nunca
// por puntero, por el mismo motivo que ya separó TokenRefrescoEmitido de su
// sesión en Acceso: confirmar un código TOTP es una operación de lectura
// pura que ocurre en cada login de un usuario con MFA activo, y cargar el
// agregado Usuario completo en cada verificación sería innecesario.
//
// Toda mutación ocurre por un método de negocio; no hay campos exportados
// ni setters, y los getters devuelven copias de valores, nunca punteros ni
// slices internos.
type FactorMFA struct {
	id              IDFactorMFA
	usuarioID       IDUsuario
	tipo            TipoFactor
	secretoCifrado  SecretoTOTPCifrado
	confirmado      bool
	activo          bool
	creadoEn        time.Time
	confirmadoEn    *time.Time
	codigosRespaldo []CodigoRespaldoMFA

	eventos []EventoDominio
}

// HabilitarFactorMFA crea un nuevo FactorMFA sin confirmar (INV-MFA-01: el
// flag Usuario.tieneMFA no se activa todavía, eso ocurre recién en
// Confirmar) y acumula el evento FactorMFAHabilitado. El secreto ya debe
// venir cifrado por CifradorSecretos: este constructor nunca ve el secreto
// en claro.
func HabilitarFactorMFA(
	id IDFactorMFA,
	usuarioID IDUsuario,
	tipo TipoFactor,
	secretoCifrado SecretoTOTPCifrado,
	ahora time.Time,
) (*FactorMFA, error) {
	if id.EsVacio() {
		return nil, &ErrIDFactorMFAInvalido{Motivo: "no puede estar vacío"}
	}
	if usuarioID.EsVacio() {
		return nil, &ErrIDUsuarioInvalido{Motivo: "no puede estar vacío"}
	}
	if tipo.EsVacio() {
		return nil, &ErrTipoFactorInvalido{Valor: ""}
	}
	if secretoCifrado.EsVacio() {
		return nil, &ErrSecretoTOTPCifradoInvalido{Motivo: "no puede estar vacío"}
	}
	f := &FactorMFA{
		id:             id,
		usuarioID:      usuarioID,
		tipo:           tipo,
		secretoCifrado: secretoCifrado,
		confirmado:     false,
		activo:         true,
		creadoEn:       ahora,
	}
	f.agregarEvento(NuevoFactorMFAHabilitado(usuarioID, id, ahora))
	return f, nil
}

// ReconstituirFactorMFA reconstruye un agregado FactorMFA a partir de datos
// ya validados y persistidos. A diferencia de HabilitarFactorMFA, no
// acumula eventos: no representa una operación de negocio nueva, sino la
// rehidratación de una ya ocurrida. activo distingue "confirmado alguna
// vez" (confirmado, que nunca vuelve a false: es un hecho histórico) de
// "vigente ahora mismo" (activo, que Deshabilitar pone en false) — sin
// esta segunda bandera, un factor deshabilitado seguiría contando para
// RepositorioFactoresMFA.ContarConfirmadosDeUsuario y bloquearía para
// siempre un HabilitarMFA posterior (ErrLimiteFactoresMFAExcedido).
func ReconstituirFactorMFA(
	id IDFactorMFA,
	usuarioID IDUsuario,
	tipo TipoFactor,
	secretoCifrado SecretoTOTPCifrado,
	confirmado bool,
	activo bool,
	creadoEn time.Time,
	confirmadoEn *time.Time,
	codigosRespaldo []CodigoRespaldoMFA,
) *FactorMFA {
	f := &FactorMFA{
		id:             id,
		usuarioID:      usuarioID,
		tipo:           tipo,
		secretoCifrado: secretoCifrado,
		confirmado:     confirmado,
		activo:         activo,
		creadoEn:       creadoEn,
	}
	if confirmadoEn != nil {
		copia := *confirmadoEn
		f.confirmadoEn = &copia
	}
	if len(codigosRespaldo) > 0 {
		f.codigosRespaldo = append([]CodigoRespaldoMFA(nil), codigosRespaldo...)
	}
	return f
}

// --- getters (copias de valor, nunca punteros ni slices internos) ----------

// ID devuelve el identificador del factor.
func (f *FactorMFA) ID() IDFactorMFA { return f.id }

// UsuarioID devuelve el identificador del Usuario dueño de este factor.
func (f *FactorMFA) UsuarioID() IDUsuario { return f.usuarioID }

// Tipo devuelve el tipo de factor (solo "totp" en el MVP, ADR 0037).
func (f *FactorMFA) Tipo() TipoFactor { return f.tipo }

// SecretoCifrado devuelve el secreto TOTP en su forma cifrada. El dominio
// nunca lo descifra por sí mismo (INV-MFA-02).
func (f *FactorMFA) SecretoCifrado() SecretoTOTPCifrado { return f.secretoCifrado }

// EstaConfirmado indica si el factor ya pasó su primera verificación
// exitosa alguna vez (INV-MFA-01). Es un hecho histórico: sigue en true
// aunque el factor se haya deshabilitado después — para saber si sigue
// vigente, ver EstaActivo.
func (f *FactorMFA) EstaConfirmado() bool { return f.confirmado }

// EstaActivo indica si el factor sigue vigente ahora mismo. Empieza en
// true al habilitarlo y pasa a false cuando Deshabilitar se ejecuta; a
// diferencia de EstaConfirmado, sí puede volver a false. Los repositorios
// deben filtrar por confirmado Y activo al contar o listar los factores
// que cuentan para INV-ID-08/INV-MFA-01 y para el límite de un factor por
// usuario del MVP (ADR 0037): sin este campo, un factor deshabilitado
// seguiría bloqueando un HabilitarMFA posterior.
func (f *FactorMFA) EstaActivo() bool { return f.activo }

// CreadoEn devuelve la marca de tiempo de creación del factor.
func (f *FactorMFA) CreadoEn() time.Time { return f.creadoEn }

// ConfirmadoEn devuelve la marca de tiempo de confirmación y un booleano
// que indica si el factor ya se confirmó.
func (f *FactorMFA) ConfirmadoEn() (time.Time, bool) {
	if f.confirmadoEn == nil {
		return time.Time{}, false
	}
	return *f.confirmadoEn, true
}

// CodigosRespaldo devuelve una copia de la colección de códigos de
// respaldo asociados a este factor (vacía si todavía no se confirmó, ADR
// 0040).
func (f *FactorMFA) CodigosRespaldo() []CodigoRespaldoMFA {
	copia := make([]CodigoRespaldoMFA, len(f.codigosRespaldo))
	copy(copia, f.codigosRespaldo)
	return copia
}

// CodigosRespaldoDisponibles cuenta cuántos códigos de respaldo no se han
// consumido todavía.
func (f *FactorMFA) CodigosRespaldoDisponibles() int {
	n := 0
	for _, c := range f.codigosRespaldo {
		if c.EstaDisponible() {
			n++
		}
	}
	return n
}

// --- mutaciones de negocio --------------------------------------------------

// Confirmar aplica la primera verificación de un FactorMFA recién habilitado
// (§1.5 y §3.2 del diseño otp-mfa.md): solo es aplicable a un factor no
// confirmado (ErrFactorMFAYaConfirmado en caso contrario) y verifica el
// código contra el secreto YA DESCIFRADO que el caso de uso le pasa — el
// dominio nunca descifra nada por sí mismo, eso es responsabilidad del
// puerto de salida CifradorSecretos. Si el código es válido, marca
// confirmado=true, confirmadoEn=ahora, y acumula el evento
// FactorMFAConfirmado (el momento exacto en que INV-ID-08/INV-MFA-01 exige
// que Usuario.tieneMFA pase a true, mediante el flip coordinado que el caso
// de uso aplica en la misma transacción vía Usuario.HabilitarMFA). Un
// código incorrecto produce ErrCodigoOTPInvalido sin mutar el agregado: no
// consume ningún intento del catálogo de dominio, el throttling es
// responsabilidad de Confianza.
func (f *FactorMFA) Confirmar(codigo CodigoTOTP, secretoDescifrado string, ahora time.Time) error {
	if f.confirmado {
		return &ErrFactorMFAYaConfirmado{}
	}
	if !VerificarCodigo(secretoDescifrado, codigo, ahora) {
		return &ErrCodigoOTPInvalido{}
	}
	f.confirmado = true
	copia := ahora
	f.confirmadoEn = &copia
	f.agregarEvento(NuevoFactorMFAConfirmado(f.usuarioID, f.id, ahora))
	return nil
}

// AsignarCodigosRespaldo adjunta los códigos de respaldo de un solo uso
// generados junto con la confirmación del factor (ADR 0040: nunca antes,
// para no dejar códigos de recuperación vivos para un factor que nunca
// llegó a confirmarse). El caso de uso ConfirmarFactorMFA invoca este
// método una única vez, inmediatamente después de que Confirmar tenga
// éxito, dentro de la misma unidad de trabajo — no existe otro punto de
// entrada del sistema que pueda invocarlo, así que el dominio no necesita
// imponer aquí una guarda adicional de "solo una vez".
func (f *FactorMFA) AsignarCodigosRespaldo(hashes []HashCodigoRespaldo) {
	codigos := make([]CodigoRespaldoMFA, 0, len(hashes))
	for _, h := range hashes {
		codigos = append(codigos, CodigoRespaldoMFA{hashCodigo: h})
	}
	f.codigosRespaldo = codigos
}

// VerificarCodigo se invoca en cada intento de step-up (§1.5 y §3.4 del
// diseño otp-mfa.md, camino caliente consumido por Acceso en cada login):
// prueba el código presentado primero contra el TOTP esperado (secreto ya
// descifrado por el caso de uso) y, si no coincide, contra los códigos de
// respaldo todavía disponibles; si uno coincide, lo marca consumido y
// acumula CodigoRespaldoConsumido. Devuelve un booleano sin distinguir en
// su forma observable cuál mecanismo (o ninguno) coincidió (INV-MFA-08). Un
// fallo total acumula VerificacionOTPFallida — señal de fuerza bruta sobre
// el segundo factor de esta cuenta.
//
// codigo se recibe como string sin pasar por el VO CodigoTOTP: en este
// punto el usuario puede presentar un TOTP de 6 dígitos o un código de
// respaldo de 10 caracteres alfanuméricos, dos formatos estructuralmente
// incompatibles con un único value object — a diferencia de Confirmar, que
// solo admite el código inicial de la app autenticadora y por eso sí tipa
// su parámetro como CodigoTOTP. Es una ambigüedad del boceto original
// (§1.2/§1.5 del diseño solo mostraban "VerificarCodigo(codigo, ahora)" sin
// precisar el tipo) resuelta aquí a favor de que el método pueda cumplir su
// contrato documentado ("si falla, contra los códigos de respaldo").
func (f *FactorMFA) VerificarCodigo(codigo string, secretoDescifrado string, ahora time.Time) bool {
	if totp, err := NuevoCodigoTOTP(codigo); err == nil {
		if VerificarCodigo(secretoDescifrado, totp, ahora) {
			return true
		}
	}
	if f.consumirCodigoRespaldoSiCoincide(codigo, ahora) {
		return true
	}
	f.agregarEvento(NuevoVerificacionOTPFallida(f.usuarioID, ahora))
	return false
}

// consumirCodigoRespaldoSiCoincide busca, entre los códigos de respaldo
// disponibles, uno cuyo hash coincida con el código presentado. Si lo
// encuentra, lo consume y acumula CodigoRespaldoConsumido (INV-MFA-06: un
// código de respaldo se consume una sola vez).
func (f *FactorMFA) consumirCodigoRespaldoSiCoincide(codigo string, ahora time.Time) bool {
	plano, err := NuevoCodigoRespaldoPlano(codigo)
	if err != nil {
		return false
	}
	hash := HashearCodigoRespaldo(plano)
	for i, c := range f.codigosRespaldo {
		if !c.EstaDisponible() || !c.HashCodigo().EsIgual(hash) {
			continue
		}
		consumido, err := c.Consumir(ahora)
		if err != nil {
			continue
		}
		f.codigosRespaldo[i] = consumido
		f.agregarEvento(NuevoCodigoRespaldoConsumido(f.usuarioID, f.id, f.CodigosRespaldoDisponibles(), ahora))
		return true
	}
	return false
}

// Deshabilitar marca el factor como no vigente (activo=false, ver
// EstaActivo) y acumula el evento FactorMFADeshabilitado (ADR 0039: el
// caso de uso ya exigió y verificó un código propio del factor antes de
// llegar aquí). No hace un DELETE físico ni necesita uno: el adaptador de
// persistencia solo tiene que guardar el agregado con Guardar, igual que
// cualquier otra mutación — activo=false es lo que hace que
// ContarConfirmadosDeUsuario deje de contarlo, permitiendo un HabilitarMFA
// posterior.
func (f *FactorMFA) Deshabilitar(ahora time.Time) {
	f.activo = false
	f.agregarEvento(NuevoFactorMFADeshabilitado(f.usuarioID, f.id, ahora))
}

// --- eventos ------------------------------------------------------------

// EventosPendientes drena los eventos acumulados por el agregado: los
// devuelve y vacía el buffer interno. El caso de uso debe llamarlo una sola
// vez, tras persistir el agregado dentro de la misma unidad de trabajo que
// la auditoría (ADR 0005).
func (f *FactorMFA) EventosPendientes() []EventoDominio {
	eventos := f.eventos
	f.eventos = nil
	return eventos
}

func (f *FactorMFA) agregarEvento(e EventoDominio) {
	f.eventos = append(f.eventos, e)
}
