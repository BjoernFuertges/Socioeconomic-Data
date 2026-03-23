#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

routes_files=(
  "internal/routes/routes.go"
  "internal/routes/routes_soziodata_generated.go"
)

extract_registered_routes() {
  awk '
    function joinPath(base, part, out) {
      out = base part
      gsub(/\/+/, "/", out)
      if (out == "") out = "/"
      if (length(out) > 1 && substr(out, length(out), 1) == "/") out = substr(out, 1, length(out)-1)
      return out
    }

    {
      if (match($0, /([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*:=[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)\.Group\("([^"]*)"\)/, m)) {
        child = m[1]
        parent = m[2]
        segment = m[3]
        base = (parent in groupPrefix) ? groupPrefix[parent] : ""
        groupPrefix[child] = joinPath(base, segment)
      }

      if (match($0, /([A-Za-z_][A-Za-z0-9_]*)\.(GET|POST|PUT|DELETE|PATCH)\("([^"]*)",[[:space:]]*(handler|h)\.([A-Za-z0-9_]+)\)/, m)) {
        groupVar = m[1]
        method = tolower(m[2])
        routePart = m[3]
        fn = m[5]

        base = (groupVar in groupPrefix) ? groupPrefix[groupVar] : ""
        path = joinPath(base, routePart)

        if (index(path, "/api/v1") == 1) {
          path = substr(path, 8)
          if (path == "") path = "/"
        }

        while (match(path, /:[A-Za-z_][A-Za-z0-9_]*/)) {
          name = substr(path, RSTART+1, RLENGTH-1)
          path = substr(path, 1, RSTART-1) "{" name "}" substr(path, RSTART+RLENGTH)
        }

        print fn "|" path "|" method
      }
    }
  ' "${routes_files[@]}"
}

get_router_annotation() {
  local handler="$1"
  local file="$2"

  awk -v fn="$handler" '
    { lines[NR] = $0 }
    $0 ~ "func \\(h \\*Handler\\) " fn "\\(.*\\*gin\\.Context\\)" {
      start = NR - 50
      if (start < 1) start = 1
      for (i = NR - 1; i >= start; i--) {
        if (match(lines[i], /@Router[[:space:]]+([^[:space:]]+)[[:space:]]+\[([^]]+)\]/, m)) {
          print m[1] "|" tolower(m[2])
          exit 0
        }
      }
      exit 1
    }
  ' "$file"
}

declare -A route_path_by_handler
declare -A route_method_by_handler

while IFS='|' read -r handler path method; do
  [[ -n "$handler" ]] || continue
  route_path_by_handler["$handler"]="$path"
  route_method_by_handler["$handler"]="$method"
done < <(extract_registered_routes)

handlers=("${!route_path_by_handler[@]}")

if [[ ${#handlers[@]} -eq 0 ]]; then
  echo "Keine Handler-Referenzen in Routen gefunden."
  exit 1
fi

missing=()
mismatch=()

for handler in "${handlers[@]}"; do
  file="$(grep -RslE "func \(h \*Handler\) ${handler}\(.*\*gin\.Context\)" internal/handlers/*.go | head -n 1 || true)"

  if [[ -z "$file" ]]; then
    missing+=("$handler (keine Handler-Funktion gefunden)")
    continue
  fi

  router_info="$(get_router_annotation "$handler" "$file" || true)"
  if [[ -z "$router_info" ]]; then
    missing+=("$handler ($file: @Router fehlt)")
    continue
  fi

  annotated_path="${router_info%%|*}"
  annotated_method="${router_info##*|}"
  expected_path="${route_path_by_handler[$handler]}"
  expected_method="${route_method_by_handler[$handler]}"

  if [[ "$annotated_path" != "$expected_path" || "$annotated_method" != "$expected_method" ]]; then
    mismatch+=("$handler ($file: erwartet ${expected_method^^} $expected_path, annotiert ${annotated_method^^} $annotated_path)")
  fi
done

if [[ ${#missing[@]} -gt 0 ]]; then
  echo "Fehlende Swagger @Router-Annotationen:"
  printf ' - %s\n' "${missing[@]}"
  exit 1
fi

if [[ ${#mismatch[@]} -gt 0 ]]; then
  echo "Swagger @Router-Annotationen stimmen nicht mit registrierten Routen überein:"
  printf ' - %s\n' "${mismatch[@]}"
  exit 1
fi

echo "Swagger @Router-Check erfolgreich (${#handlers[@]} Handler inkl. Pfad/HTTP-Methode geprüft)."
