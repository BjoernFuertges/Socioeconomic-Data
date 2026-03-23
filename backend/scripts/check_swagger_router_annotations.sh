#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

routes_files=(
  "internal/routes/routes.go"
  "internal/routes/routes_soziodata_generated.go"
)

extract_registered_routes() {
  awk '
    function trim(s) {
      sub(/^[ \t]+/, "", s)
      sub(/[ \t]+$/, "", s)
      return s
    }

    function joinPath(base, part) {
      result = base part
      while (index(result, "//") > 0) {
        slash_pos = index(result, "//")
        result = substr(result, 1, slash_pos) substr(result, slash_pos + 2)
      }
      if (result == "") result = "/"
      if (length(result) > 1) {
        lastchar = substr(result, length(result), 1)
        if (lastchar == "/") {
          result = substr(result, 1, length(result) - 1)
        }
      }
      return result
    }

    function normalizePath(path) {
      if (index(path, "/api/v1") == 1) {
        path = substr(path, 8)
        if (path == "") path = "/"
      }

      while (match(path, /:[A-Za-z_][A-Za-z0-9_]*/)) {
        name = substr(path, RSTART + 1, RLENGTH - 1)
        path = substr(path, 1, RSTART - 1) "{" name "}" substr(path, RSTART + RLENGTH)
      }

      return path
    }

    {
      if ($0 ~ / := .*\.Group\(/) {
        line = $0
        var_part = line
        sub(/:=.*/, "", var_part)
        var = trim(var_part)

        group_part = line
        sub(/.*\.Group\("/, "", group_part)
        sub(/".*/, "", group_part)
        group_path = group_part

        parent_part = line
        sub(/.*:= /, "", parent_part)
        sub(/\.Group.*/, "", parent_part)
        parent = trim(parent_part)

        if (parent in groupPrefix) {
          base = groupPrefix[parent]
        } else {
          base = ""
        }
        groupPrefix[var] = joinPath(base, group_path)
      }

      if ($0 ~ /\.(GET|POST|PUT|DELETE|PATCH)\(/) {
        line = $0

        method_part = line
        sub(/\./, " ", method_part)
        sub(/^[^ ]*[ ]/, "", method_part)
        sub(/\(.*/, "", method_part)
        method = tolower(method_part)

        groupVar_part = line
        sub(/\..*/, "", groupVar_part)
        groupVar = trim(groupVar_part)

        route_part = line
        sub(/.*\("/, "", route_part)
        sub(/".*/, "", route_part)
        routePart = route_part

        fn_part = line
        if (index(fn_part, "handler.") > 0 || index(fn_part, "h.") > 0) {
          sub(/.*handler\./, "", fn_part)
          if (length(fn_part) == length(line)) {
            fn_part = line
            sub(/.*h\./, "", fn_part)
          }
          sub(/[,) ].*/, "", fn_part)
          fn = trim(fn_part)
        } else {
          fn = ""
        }

        if (fn != "" && method != "" && routePart != "") {
          if (groupVar in groupPrefix) {
            base = groupPrefix[groupVar]
          } else {
            base = ""
          }
          path = normalizePath(joinPath(base, routePart))

          print fn "|" path "|" method
        }
      }
    }
  ' "${routes_files[@]}"
}

normalize_comparable_path() {
  local path="$1"

  if [[ "$path" == /api/v1* ]]; then
    path="${path#/api/v1}"
    [[ -n "$path" ]] || path="/"
  fi

  printf '%s\n' "$path"
}

get_router_annotation() {
  local handler="$1"
  local file="$2"

  awk -v fn="$handler" '
    { lines[NR] = $0 }
    END {
      found = 0
      for (i = 1; i <= NR; i++) {
        if (lines[i] ~ "func.*Handler.*" fn) {
          start = i - 50
          if (start < 1) start = 1
          for (j = i - 1; j >= start; j--) {
            if (index(lines[j], "@Router") > 0) {
              line = lines[j]
              rest = line
              sub(/.*@Router[ ]*/, "", rest)
              
              path = rest
              sub(/[ ].*/, "", path)
              
              method_part = rest
              sub(/^[^ ]*[ ]*/, "", method_part)
              sub(/\[/, "", method_part)
              sub(/\].*/, "", method_part)
              
              if (length(path) > 0 && length(method_part) > 0) {
                print path "|" tolower(method_part)
                found = 1
                break
              }
            }
          }
          if (found) break
        }
      }
      if (!found) exit 1
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

  annotated_path="$(normalize_comparable_path "$annotated_path")"
  expected_path="$(normalize_comparable_path "$expected_path")"

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
