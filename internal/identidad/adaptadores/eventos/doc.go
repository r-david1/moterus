// Package eventos implementa puertos.PublicadorEventos para la
// integración asíncrona (correo de bienvenida, métricas, etc). El stack
// fijo del proyecto no incluye todavía un message broker (Kafka/NATS no
// están en el stack — ver colas-virtuales sobre Redis, agente 08, fuera de
// alcance de este adapter): la única implementación disponible hoy es
// PublicadorLog, que registra los eventos en el logger estructurado y no
// entrega nada a un tercero. Ver publicador.go para el detalle y la
// limitación documentada.
package eventos
