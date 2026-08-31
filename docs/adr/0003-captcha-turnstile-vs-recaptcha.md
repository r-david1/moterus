# ADR 0003 — Captcha invisible: Cloudflare Turnstile por defecto

## Contexto

Se requiere captcha invisible (basado en score, sin fricción para el usuario) para proteger login/registro. La primera opción evaluada fue **reCAPTCHA v3**, con la pregunta explícita de si tiene un tier gratuito viable.

## Investigación (con fuentes de 2026)

- Desde el 1 de abril de 2024, Google cambió el modelo de precios de reCAPTCHA: el tier gratuito bajó de 1,000,000 a **10,000 evaluaciones/mes** ("reCAPTCHA-lite").
- Sobre las 10,000 gratis, el plan "estándar" cuesta $8/mes hasta 100,000 evaluaciones; después, tarifa por cada 1,000 adicionales.
- Desde 2025-2026, Google ya no permite crear **keys clásicas nuevas** — todo pasa por un proyecto de Google Cloud, con facturación asociada a esa cuenta (aunque te mantengas en el tier gratis).

## Decisión

Usar **Cloudflare Turnstile** como proveedor por defecto: compatible con la misma API cliente/servidor de reCAPTCHA v2/v3 (mismo flujo de verificación), invisible, gratis hasta un volumen mucho más alto (del orden de 1 millón/mes), sin necesidad de cuenta de Google Cloud ni cookies de tracking de Google.

Se diseña el puerto `CaptchaVerifier` como interfaz agnóstica del proveedor para poder cambiar sin tocar casos de uso.

## Alternativas consideradas

- **reCAPTCHA v3 gratis**: viable igualmente si el proyecto es de un solo producto con tráfico bajo/medio (10,000/mes ≈ 333/día puede alcanzar). Se descartó como default porque exige gestionar un proyecto de Google Cloud adicional y el techo es más bajo; queda documentado como alternativa intercambiable, no descartada por completo.
- **hCaptcha**: no evaluado a fondo en esta ronda — queda como opción a investigar si Turnstile no encaja por algún motivo (ej. bloqueo regional de Cloudflare).

## Consecuencias

- No se paga nada por captcha en el volumen actual del proyecto.
- Si se retoma el escenario multi-producto (ver ADR 0002) y el volumen agregado crece, revisar si Turnstile sigue siendo suficiente o si conviene reevaluar.

## Estado

Aceptado.
