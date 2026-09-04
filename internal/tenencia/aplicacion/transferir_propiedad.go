package aplicacion

import (
	"context"
	"log/slog"

	"github.com/r-david1/moterus/internal/tenencia/dominio"
	"github.com/r-david1/moterus/internal/tenencia/puertos"
)

// TransferirPropiedad implementa el caso de uso homónimo (§3.2 del diseño).
// dominio.Membresia no expone un método propio "TransferirPropiedad" (no
// aparece en el diagrama de clases de §1.1 del diseño): la operación se
// modela aquí como la composición de dos mutaciones de CambiarRol sobre dos
// agregados Membresia distintos, en la misma UnidadDeTrabajo y EN ESTE
// ORDEN — promover primero, degradar después — que es lo que hace que el
// invariante del último propietario (INV-TEN-06) se sostenga incluso
// intra-transacción, sin depender del CONSTRAINT TRIGGER diferido como
// única red de seguridad:
//
//  1. Autorizar con propiedad.transferir: en el catálogo cerrado de
//     matrizDePermisos solo RolPropietario tiene este permiso, así que el
//     rol efectivo devuelto por Autorizar es siempre "propietario" — no
//     hace falta comprobarlo aparte.
//  2. Candado de fila + comprobar que la organización sigue operativa.
//  3. Resolver la membresía del destinatario: debe ser miembro ACTIVO
//     previo (§3.2 del diseño). Ni "no es miembro" ni "es miembro pero está
//     suspendido" son un destinatario válido para una transferencia de
//     propiedad; dominio no distingue un error propio para "suspendido" en
//     este contexto, así que ambos casos se tratan como
//     ErrMembresiaNoEncontrada.
//  4. Promover al destinatario a propietario (CambiarRol). Se calcula el
//     conteo de propietarios ANTES de la promoción y, si el destinatario no
//     era ya propietario (transferir a un co-propietario existente es un
//     no-op de promoción), se incrementa en uno para la comprobación de la
//     siguiente mutación: así nunca se consulta la base de datos una
//     segunda vez para el mismo invariante dentro de la misma transacción.
//  5. Si RolResultanteDelCedente no es "" (cadena vacía = conservar
//     propietario, co-propiedad explícita), degradar al cedente
//     (CambiarRol) con el conteo ya actualizado por el paso anterior.
func (c *MembresiasCasoDeUso) TransferirPropiedad(ctx context.Context, cmd puertos.ComandoTransferirPropiedad) error {
	idOrganizacion, err := dominio.IDOrganizacionDesde(cmd.IDOrganizacion)
	if err != nil {
		return err
	}
	idCedente, err := dominio.IDUsuarioDesde(cmd.IDSujeto)
	if err != nil {
		return err
	}
	idDestinatario, err := dominio.IDUsuarioDesde(cmd.IDNuevoPropietario)
	if err != nil {
		return err
	}

	conservarPropiedad := cmd.RolResultanteDelCedente == ""
	var rolResultanteCedente dominio.Rol
	if !conservarPropiedad {
		rolResultanteCedente, err = dominio.RolDesde(cmd.RolResultanteDelCedente)
		if err != nil {
			return err
		}
	}

	decision, err := autorizar(ctx, c.autorizador, cmd.IDSujeto, cmd.IDOrganizacion, dominio.PermisoPropiedadTransferir, cmd.Origen)
	if err != nil {
		return err
	}
	rolEjecutor, err := dominio.RolDesde(decision.Rol) // siempre "propietario"
	if err != nil {
		return err
	}

	var eventos []dominio.EventoDominio
	if err := c.uow.Ejecutar(ctx, func(ctx context.Context) error {
		org, err := c.organizaciones.CargarParaActualizar(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		if org == nil {
			return &dominio.ErrOrganizacionNoEncontrada{ID: cmd.IDOrganizacion}
		}
		if err := org.EstaOperativa(); err != nil {
			return err
		}

		destinatario, err := c.membresias.BuscarVigente(ctx, idDestinatario, idOrganizacion)
		if err != nil {
			return err
		}
		if destinatario == nil || !destinatario.Estado().EsIgual(dominio.EstadoMembresiaActiva) {
			return &dominio.ErrMembresiaNoEncontrada{}
		}

		ahora := c.reloj.Ahora()
		propietariosActivos, err := c.membresias.ContarPropietariosActivos(ctx, idOrganizacion)
		if err != nil {
			return err
		}
		yaEraPropietario := destinatario.Rol().EsIgual(dominio.RolPropietario)

		if err := destinatario.CambiarRol(dominio.RolPropietario, rolEjecutor, propietariosActivos, ahora); err != nil {
			return err
		}
		propietariosActivosTrasPromocion := propietariosActivos
		if !yaEraPropietario {
			propietariosActivosTrasPromocion++
		}
		if err := c.membresias.Guardar(ctx, destinatario); err != nil {
			return err
		}
		eventos = append(eventos, destinatario.EventosPendientes()...)

		if !conservarPropiedad {
			cedente, err := c.membresias.BuscarVigente(ctx, idCedente, idOrganizacion)
			if err != nil {
				return err
			}
			if cedente == nil {
				return &dominio.ErrMembresiaNoEncontrada{}
			}
			if err := cedente.CambiarRol(rolResultanteCedente, dominio.RolPropietario, propietariosActivosTrasPromocion, ahora); err != nil {
				return err
			}
			if err := c.membresias.Guardar(ctx, cedente); err != nil {
				return err
			}
			eventos = append(eventos, cedente.EventosPendientes()...)
		}

		for _, e := range eventos {
			if err := c.auditoria.Registrar(ctx, e, idOrganizacion, cmd.Origen); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if len(eventos) > 0 {
		if errPub := c.eventos.Publicar(ctx, eventos...); errPub != nil {
			slog.WarnContext(ctx, "transferir propiedad: fallo al publicar eventos",
				"error", errPub, "organizacion_id", idOrganizacion.String())
		}
	}
	return nil
}
