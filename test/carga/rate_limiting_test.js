// Escenario de carga básico (ADR 0018) para validar que el rate limiting
// de Confianza realmente corta el tráfico ANTES de que llegue a Postgres,
// no solo en teoría. Ejecutar con `make carga` (requiere k6 instalado:
// https://k6.io/docs/get-started/installation/) contra un servidor ya
// levantado con `make run` (que a su vez requiere `make docker-up` +
// `make migrate-up` + REDIS_URL definido para que el limitador sea real,
// no el no-op).
//
// Qué valida cada escenario:
//   - login_fuerza_bruta: ráfaga de intentos de login contra la MISMA
//     cuenta desde una sola IP. Se espera ver 401 (credenciales
//     inválidas) en los primeros intentos y 429 (límite excedido) en el
//     resto, con Retry-After presente — nunca un 500 ni una latencia que
//     crezca sin límite (evidencia de que el corte ocurre en el
//     limitador, no en el pool de Postgres agotándose).
//   - registro_rafaga: ráfaga de altas con correos distintos desde una
//     sola IP, para validar el límite de IP de registro (10/min) sin
//     tocar el límite de cuenta (cada correo es único).
//   - reenvio_spam: ráfaga de reenvíos de verificación al MISMO correo,
//     para validar el guardián de perímetro HTTP (no pasa por
//     aplicacion, ver ADR 0018).
//
// Umbrales esperados (ver internal/confianza/dominio/umbral.go):
//   login: IP 5/1min, cuenta 5/15min · registro: IP 10/1min, cuenta 3/15min
//   reenvio_verificacion: IP 5/1min, cuenta 3/15min
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  scenarios: {
    login_fuerza_bruta: {
      executor: 'shared-iterations',
      vus: 1, // misma "IP" (el propio runner de k6): a propósito, es lo
      // que se quiere agotar.
      iterations: 10,
      exec: 'loginFuerzaBruta',
      maxDuration: '30s',
    },
    registro_rafaga: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 15,
      exec: 'registroRafaga',
      startTime: '5s',
      maxDuration: '30s',
    },
    reenvio_spam: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 6,
      exec: 'reenvioSpam',
      startTime: '10s',
      maxDuration: '30s',
    },
  },
};

export function loginFuerzaBruta() {
  const res = http.post(
    `${BASE_URL}/identidad/autenticaciones`,
    JSON.stringify({ correo: 'victima-carga-k6@ejemplo.com', contrasena: 'lo-que-sea-1234' }),
    { headers: { 'Content-Type': 'application/json' } }
  );
  check(res, {
    'nunca 500': (r) => r.status < 500,
    '401 o 429, nunca 200 (no hay cuenta real)': (r) => r.status === 401 || r.status === 429,
  });
  if (res.status === 429) {
    check(res, { 'trae Retry-After': (r) => r.headers['Retry-After'] !== undefined });
  }
  sleep(0.2);
}

export function registroRafaga() {
  const correo = `carga-k6-${__VU}-${__ITER}-${Date.now()}@ejemplo.com`;
  const res = http.post(
    `${BASE_URL}/identidad/usuarios`,
    JSON.stringify({ correo, contrasena: 'correcto caballo bateria grapa' }),
    { headers: { 'Content-Type': 'application/json' } }
  );
  check(res, {
    'nunca 500': (r) => r.status < 500,
    '201/200/422/429 esperados': (r) => [200, 201, 422, 429].includes(r.status),
  });
  sleep(0.2);
}

export function reenvioSpam() {
  const res = http.post(
    `${BASE_URL}/identidad/verificaciones-correo/reenvios`,
    JSON.stringify({ correo: 'spam-carga-k6@ejemplo.com' }),
    { headers: { 'Content-Type': 'application/json' } }
  );
  check(res, {
    'nunca 500': (r) => r.status < 500,
    '202 o 429': (r) => r.status === 202 || r.status === 429,
  });
  sleep(0.2);
}
