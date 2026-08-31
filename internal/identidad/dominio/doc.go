// Package dominio contiene el agregado Usuario, sus value objects, servicios
// de dominio, eventos y errores del bounded context Identidad.
//
// Regla de arquitectura (INV-ID-18): este paquete no importa nada fuera de
// la stdlib de Go (permitido: time, strings, errors, fmt, net, net/mail,
// unicode, unicode/utf8, crypto/subtle, etc.). Cero dependencias externas,
// cero imports de internal/plataforma o de cualquier otro paquete del
// proyecto.
package dominio
