// Package cache contiene el cliente Redis compartido (go-redis/v9)
// usado por rate limiting, colas virtuales y cachés de lectura. Hoy lo
// consume el bounded context Confianza (internal/confianza/adaptadores/redis)
// para el limitador de tasa; es kernel técnico puro — no conoce reglas de
// negocio de ningún contexto.
package cache
