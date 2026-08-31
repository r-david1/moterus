// Package http contiene los handlers Huma v2 (sobre Fiber v2 vía
// humafiber.NewV2), los DTOs y el mapeo de errores de dominio a códigos
// HTTP del bounded context Identidad (ADR 0006). Expone los tres endpoints
// del MVP: registro, autenticación (sin emitir JWT, ADR 0009) y consulta de
// usuario — ver rutas.go para el detalle y el nivel de auth de cada uno.
package http
