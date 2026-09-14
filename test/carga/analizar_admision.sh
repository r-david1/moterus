#!/usr/bin/env bash
# Analiza el JSON de salida de k6 (--out json=<archivo>) para verificar el
# criterio (a) de docs/design/colas-virtuales.md §11: el endpoint protegido
# nunca admite más de ~ritmo peticiones/segundo. k6 no tiene un threshold
# nativo de "conteo por ventana de 1s", así que se agrupa por segundo de
# reloj con jq sobre las peticiones ya filtradas por la etiqueta
# admision:"protegido_real" que colas_virtuales_test.js les pone — el
# mismo tráfico que la Consulta de turno o el ingreso NO llevan esa
# etiqueta y quedan fuera del conteo.
#
# Uso: bash test/carga/analizar_admision.sh resultados_colas.json [ritmo]
#   ritmo (opcional, default 50) es el ritmo_admision configurado en
#   abrir_sala_carga.sql.
set -euo pipefail

archivo="${1:?uso: analizar_admision.sh <resultados.json> [ritmo]}"
ritmo="${2:-50}"
# 20% de margen sobre el ritmo nominal: la agrupación es por segundo de
# reloj de pared, no por la ventana deslizante exacta que usa el cursor del
# servidor, así que un efecto de borde entre dos segundos consecutivos es
# esperable — no una señal de que el middleware falló en limitar.
tolerancia=$((ritmo * 12 / 10))

echo "Admisiones reales al endpoint protegido (acceso.iniciar_sesion), por segundo:"
echo

conteos="$(jq -r '
  select(.metric=="http_reqs" and .data.tags.admision=="protegido_real" and .data.tags.status != "503")
  | .data.time[0:19]
' "$archivo" | sort | uniq -c | sort -rn)"

echo "$conteos" | head -15
echo "..."
echo

max="$(echo "$conteos" | awk '{print $1}' | sort -rn | head -1)"
max="${max:-0}"

echo "Ritmo configurado: ${ritmo}/s · tolerancia (borde de segundo): ${tolerancia}/s · máximo observado: ${max}/s"
echo
echo "Nota de lectura si el máximo aparece muy por encima del ritmo, concentrado"
echo "en uno o dos segundos cerca del final de la corrida: es un artefacto de"
echo "sondeo del CLIENTE, no necesariamente del servidor. reconsultar_en_ms crece"
echo "con la posición (por diseño, para no bombardear al servidor); un cliente"
echo "con un intervalo grande puede descubrir que ya fue admitido mucho después"
echo "de que el cursor del servidor realmente lo alcanzó, y llamar al endpoint"
echo "real tarde — si muchos clientes comparten un intervalo de sondeo parecido,"
echo "sus llamadas 'tardías' se agrupan. Este efecto se vuelve más notorio cuanto"
echo "más se comprime el drenaje (ritmo alto / pocos usuarios); a la escala"
echo "documentada en docs/design/colas-virtuales.md §11 (5000 usuarios, drenaje"
echo "~100s) hay más tiempo real entre sondeos y el efecto se diluye. Si esto es"
echo "lo que se está viendo, confirmarlo mirando si el pico coincide con el final"
echo "de la corrida (dropped_iterations/iteraciones que tardaron el máximo de"
echo "esperaMaximaSegundos) antes de concluir que el middleware falló."

if [ "$max" -gt "$tolerancia" ]; then
  echo "FALLA: el máximo observado (${max}/s) supera el ritmo configurado con margen (${tolerancia}/s)."
  echo "El middleware de sala de espera no está acotando el tráfico al endpoint protegido como se espera."
  exit 1
fi

echo "OK: ninguna ventana de 1s superó el ritmo configurado con margen."
