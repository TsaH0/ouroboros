#!/usr/bin/env bash
# Ouroboros Advisor - Layered HTTP Security Traffic Triaging & Analysis Tool
# Allows AI agents (Claude Code, Antigravity, LLMs) and users to progressively inspect,
# triage, filter, and audit HTTP traffic captured by Ouroboros.
#
# Layers:
#   Layer 1: Overview & Flow Index (overview, presets, hosts, flows)
#   Layer 2: Filtered Search & Discovery (filter, endpoints, params)
#   Layer 3: Progressive Security Triaging (triage, triage-injections, triage-params, triage-errors, triage-auth, suspicious)
#   Layer 4: Deep Payload & Flow Inspection (flow, body, headers, curl, diff)
#   Layer 5: AI Analyses & Advisory (analyses, analysis, advise)

set -euo pipefail

DB="${OUROBOROS_DB:-$HOME/.config/ouroboros/ouroboros.db}"

if [[ ! -f "$DB" ]]; then
  echo >&2 "Error: Ouroboros database not found at $DB"
  echo >&2 "Run Ouroboros first, or set OUROBOROS_DB=/path/to/ouroboros.db"
  exit 1
fi

if ! command -v sqlite3 &>/dev/null; then
  echo >&2 "Error: sqlite3 is not installed."
  echo >&2 "Install with: sudo pacman -S sqlite   (Arch)"
  echo >&2 "         or: sudo apt install sqlite3  (Debian/Ubuntu)"
  exit 1
fi

cmd="${1:-overview}"
shift || true

# Helper for parsing optional named flags
parse_flag() {
  local flag="$1"
  local default_val="$2"
  shift 2
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "$flag" && -n "${2:-}" ]]; then
      echo "$2"
      return 0
    fi
    shift
  done
  echo "$default_val"
}

case "$cmd" in
  # ══════════════════════════════════════════════════════════════════════════════
  # LAYER 1: PROJECT & TRAFFIC OVERVIEW
  # ══════════════════════════════════════════════════════════════════════════════

  overview|stats)
    echo "========================================================"
    echo "  Ouroboros Proxy — Security Traffic Overview"
    echo "  Database: $DB"
    echo "========================================================"
    echo ""
    echo "--- Traffic Counts ---"
    sqlite3 "$DB" -separator ': ' "
      SELECT 'Total Flows Captured', COUNT(*) FROM flows
      UNION ALL
      SELECT 'Unique Target Hosts', COUNT(DISTINCT host) FROM flows WHERE host != ''
      UNION ALL
      SELECT 'In-Scope Flows', COUNT(*) FROM flows WHERE scope_status = 'in_scope'
      UNION ALL
      SELECT 'Out-of-Scope Flows', COUNT(*) FROM flows WHERE scope_status = 'out_of_scope'
      UNION ALL
      SELECT 'Unknown Scope Flows', COUNT(*) FROM flows WHERE scope_status = 'unknown' OR scope_status = ''
      UNION ALL
      SELECT 'Intercepted/Pending', COUNT(*) FROM flows WHERE state = 'intercepted' OR state = 'pending'
      UNION ALL
      SELECT 'AI Security Analyses', COUNT(*) FROM analyses
    "
    echo ""
    echo "--- Status Code Breakdown ---"
    sqlite3 -header -column "$DB" "
      SELECT
        CASE
          WHEN resp_status >= 200 AND resp_status < 300 THEN '2xx (Success)'
          WHEN resp_status >= 300 AND resp_status < 400 THEN '3xx (Redirect)'
          WHEN resp_status >= 400 AND resp_status < 500 THEN '4xx (Client Error)'
          WHEN resp_status >= 500 THEN '5xx (Server Error)'
          ELSE 'No Response / Pending'
        END AS status_category,
        COUNT(*) AS flow_count
      FROM flows
      GROUP BY status_category
      ORDER BY flow_count DESC;
    "
    echo ""
    echo "--- Scope Presets ---"
    sqlite3 -header -column "$DB" "
      SELECT p.name AS preset_name, COUNT(r.id) AS rule_count, p.created_at
      FROM scope_presets p
      LEFT JOIN scope_rules r ON r.preset_id = p.id
      GROUP BY p.id
      UNION ALL
      SELECT 'Global (Unassigned)' AS preset_name, COUNT(id) AS rule_count, '-'
      FROM scope_rules WHERE preset_id = '' OR preset_id IS NULL;
    " 2>/dev/null || echo "No presets table found (schema v1)."
    ;;

  presets)
    sqlite3 -json "$DB" "
      SELECT id, name, created_at, updated_at FROM scope_presets ORDER BY created_at
    " 2>/dev/null || echo "[]"
    ;;

  hosts)
    sqlite3 -json "$DB" "
      SELECT
        host,
        COUNT(*) AS flow_count,
        COUNT(CASE WHEN scope_status = 'in_scope' THEN 1 END) AS in_scope_count,
        COUNT(CASE WHEN scope_status = 'out_of_scope' THEN 1 END) AS out_of_scope_count,
        COUNT(CASE WHEN resp_status >= 500 THEN 1 END) AS server_errors,
        COUNT(CASE WHEN resp_status = 401 OR resp_status = 403 THEN 1 END) AS auth_errors,
        MIN(datetime(start_time)) AS first_seen,
        MAX(datetime(start_time)) AS last_seen
      FROM flows
      WHERE host != ''
      GROUP BY host
      ORDER BY flow_count DESC;
    "
    ;;

  flows|list)
    limit="$(parse_flag --limit 100 "$@")"
    scope_arg="$(parse_flag --scope "" "$@")"
    host_arg="$(parse_flag --host "" "$@")"
    method_arg="$(parse_flag --method "" "$@")"
    status_arg="$(parse_flag --status "" "$@")"

    where="1=1"
    [[ -n "$scope_arg" ]] && where="$where AND scope_status = '$scope_arg'"
    [[ -n "$host_arg" ]] && where="$where AND host LIKE '%$host_arg%'"
    [[ -n "$method_arg" ]] && where="$where AND req_method = '$method_arg'"
    [[ -n "$status_arg" ]] && where="$where AND resp_status = '$status_arg'"

    sqlite3 -json "$DB" "
      SELECT
        id,
        req_method,
        host,
        req_url,
        resp_status,
        scope_status,
        duration_ns / 1000000 AS duration_ms,
        state,
        datetime(start_time) AS captured_at
      FROM flows
      WHERE $where
      ORDER BY start_time DESC
      LIMIT $limit;
    "
    ;;

  # ══════════════════════════════════════════════════════════════════════════════
  # LAYER 2: FILTERED DISCOVERY & ATTACK SURFACE MAPPING
  # ══════════════════════════════════════════════════════════════════════════════

  filter|search)
    query_text="$(parse_flag --search "" "$@")"
    host_arg="$(parse_flag --host "" "$@")"
    path_arg="$(parse_flag --path "" "$@")"
    method_arg="$(parse_flag --method "" "$@")"
    status_arg="$(parse_flag --status "" "$@")"
    scope_arg="$(parse_flag --scope "" "$@")"
    limit="$(parse_flag --limit 100 "$@")"

    where="1=1"
    [[ -n "$host_arg" ]] && where="$where AND host LIKE '%$host_arg%'"
    [[ -n "$path_arg" ]] && where="$where AND req_url LIKE '%$path_arg%'"
    [[ -n "$method_arg" ]] && where="$where AND req_method = '$method_arg'"
    [[ -n "$status_arg" ]] && where="$where AND resp_status = '$status_arg'"
    [[ -n "$scope_arg" ]] && where="$where AND scope_status = '$scope_arg'"
    if [[ -n "$query_text" ]]; then
      where="$where AND (req_url LIKE '%$query_text%' OR CAST(req_body AS TEXT) LIKE '%$query_text%' OR CAST(resp_body AS TEXT) LIKE '%$query_text%' OR req_headers LIKE '%$query_text%')"
    fi

    sqlite3 -json "$DB" "
      SELECT
        id,
        req_method,
        host,
        req_url,
        resp_status,
        scope_status,
        duration_ns / 1000000 AS duration_ms,
        datetime(start_time) AS captured_at
      FROM flows
      WHERE $where
      ORDER BY start_time DESC
      LIMIT $limit;
    "
    ;;

  endpoints)
    host_arg="$(parse_flag --host "" "$@")"
    where="1=1"
    [[ -n "$host_arg" ]] && where="$where AND host LIKE '%$host_arg%'"

    sqlite3 -json "$DB" "
      SELECT
        host,
        req_method,
        substr(req_url, 1, case instr(req_url, '?') when 0 then length(req_url) else instr(req_url, '?')-1 end) AS endpoint_path,
        COUNT(*) AS request_count,
        GROUP_CONCAT(DISTINCT resp_status) AS status_codes,
        scope_status
      FROM flows
      WHERE $where AND req_method IS NOT NULL AND req_method != ''
      GROUP BY host, req_method, endpoint_path
      ORDER BY request_count DESC;
    "
    ;;

  params)
    host_arg="$(parse_flag --host "" "$@")"
    where="req_url LIKE '%?%'"
    [[ -n "$host_arg" ]] && where="$where AND host LIKE '%$host_arg%'"

    sqlite3 -json "$DB" "
      SELECT
        id,
        host,
        req_method,
        substr(req_url, instr(req_url, '?') + 1) AS query_string,
        resp_status,
        scope_status
      FROM flows
      WHERE $where
      ORDER BY start_time DESC
      LIMIT 100;
    "
    ;;

  # ══════════════════════════════════════════════════════════════════════════════
  # LAYER 3: PROGRESSIVE SECURITY & ANOMALY TRIAGING
  # ══════════════════════════════════════════════════════════════════════════════

  triage)
    echo "========================================================"
    echo "  Ouroboros Progressive Security Triage Summary"
    echo "========================================================"
    echo ""
    echo "[1] Potential Injection & Payload Matches:"
    sqlite3 -header -column "$DB" "
      SELECT COUNT(*) AS count, 'Injection / Attack Signatures in URLs or Bodies' AS category
      FROM flows
      WHERE
        lower(req_url) LIKE '%union%' OR lower(req_url) LIKE '%select%' OR lower(req_url) LIKE '%script%'
        OR lower(req_url) LIKE '%169.254.169.254%' OR lower(req_url) LIKE '%localhost%' OR lower(req_url) LIKE '%127.0.0.1%'
        OR lower(req_url) LIKE '%../%' OR lower(req_url) LIKE '%..%2f%' OR lower(req_url) LIKE '%etc/passwd%'
        OR lower(CAST(req_body AS TEXT)) LIKE '%union%select%' OR lower(CAST(req_body AS TEXT)) LIKE '%<script%'
    "
    echo ""
    echo "[2] High-Risk Parameter Patterns (SSRF/IDOR/Redirect/PrivEsc):"
    sqlite3 -header -column "$DB" "
      SELECT COUNT(*) AS count, 'Sensitive parameter names (redirect, token, admin, file, cmd, url)' AS category
      FROM flows
      WHERE
        lower(req_url) LIKE '%redirect=%' OR lower(req_url) LIKE '%url=%' OR lower(req_url) LIKE '%next=%'
        OR lower(req_url) LIKE '%admin=%' OR lower(req_url) LIKE '%role=%' OR lower(req_url) LIKE '%privilege=%'
        OR lower(req_url) LIKE '%token=%' OR lower(req_url) LIKE '%secret=%' OR lower(req_url) LIKE '%key=%'
        OR lower(req_url) LIKE '%file=%' OR lower(req_url) LIKE '%path=%' OR lower(req_url) LIKE '%cmd=%'
    "
    echo ""
    echo "[3] Server Errors & Information Leaks (5xx):"
    sqlite3 -header -column "$DB" "
      SELECT COUNT(*) AS count, '5xx Internal Server Errors' AS category
      FROM flows WHERE resp_status >= 500;
    "
    echo ""
    echo "[4] Authentication & Access Control Anomalies (401/403):"
    sqlite3 -header -column "$DB" "
      SELECT COUNT(*) AS count, '401 Unauthorized / 403 Forbidden flows' AS category
      FROM flows WHERE resp_status IN (401, 403);
    "
    echo ""
    echo "[5] Sensitive Endpoints Access:"
    sqlite3 -header -column "$DB" "
      SELECT COUNT(*) AS count, 'Sensitive path probes (.git, .env, swagger, actuator, debug)' AS category
      FROM flows
      WHERE
        lower(req_url) LIKE '%.git%' OR lower(req_url) LIKE '%.env%'
        OR lower(req_url) LIKE '%actuator%' OR lower(req_url) LIKE '%swagger%'
        OR lower(req_url) LIKE '%/metrics%' OR lower(req_url) LIKE '%/debug%';
    "
    echo ""
    echo "Tip: Run 'query.sh triage-injections', 'query.sh triage-params', or 'query.sh triage-errors' for JSON details."
    ;;

  triage-injections)
    sqlite3 -json "$DB" "
      SELECT
        id,
        host,
        req_method,
        req_url,
        resp_status,
        scope_status,
        CASE
          WHEN lower(req_url) LIKE '%union%' OR lower(req_url) LIKE '%select%' OR lower(CAST(req_body AS TEXT)) LIKE '%union%select%' THEN 'SQL_INJECTION_PATTERN'
          WHEN lower(req_url) LIKE '%script%' OR lower(CAST(req_body AS TEXT)) LIKE '%<script%' THEN 'XSS_PATTERN'
          WHEN lower(req_url) LIKE '%169.254.169.254%' OR lower(req_url) LIKE '%localhost%' OR lower(req_url) LIKE '%127.0.0.1%' THEN 'SSRF_PATTERN'
          WHEN lower(req_url) LIKE '%../%' OR lower(req_url) LIKE '%..%2f%' OR lower(req_url) LIKE '%etc/passwd%' THEN 'PATH_TRAVERSAL_PATTERN'
          ELSE 'ANOMALOUS_PAYLOAD'
        END AS detected_threat_category,
        datetime(start_time) AS captured_at
      FROM flows
      WHERE
        lower(req_url) LIKE '%union%' OR lower(req_url) LIKE '%select%' OR lower(req_url) LIKE '%script%'
        OR lower(req_url) LIKE '%169.254.169.254%' OR lower(req_url) LIKE '%localhost%' OR lower(req_url) LIKE '%127.0.0.1%'
        OR lower(req_url) LIKE '%../%' OR lower(req_url) LIKE '%..%2f%' OR lower(req_url) LIKE '%etc/passwd%'
        OR lower(CAST(req_body AS TEXT)) LIKE '%union%select%' OR lower(CAST(req_body AS TEXT)) LIKE '%<script%'
      ORDER BY start_time DESC
      LIMIT 100;
    "
    ;;

  triage-params)
    sqlite3 -json "$DB" "
      SELECT
        id,
        host,
        req_method,
        req_url,
        resp_status,
        scope_status,
        CASE
          WHEN lower(req_url) LIKE '%redirect=%' OR lower(req_url) LIKE '%url=%' OR lower(req_url) LIKE '%next=%' THEN 'OPEN_REDIRECT_OR_SSRF'
          WHEN lower(req_url) LIKE '%admin=%' OR lower(req_url) LIKE '%role=%' OR lower(req_url) LIKE '%privilege=%' THEN 'PRIVILEGE_ESCALATION'
          WHEN lower(req_url) LIKE '%token=%' OR lower(req_url) LIKE '%secret=%' OR lower(req_url) LIKE '%key=%' THEN 'SENSITIVE_CREDENTIAL'
          WHEN lower(req_url) LIKE '%file=%' OR lower(req_url) LIKE '%path=%' THEN 'LOCAL_FILE_INCLUSION'
          WHEN lower(req_url) LIKE '%cmd=%' OR lower(req_url) LIKE '%exec=%' THEN 'COMMAND_EXECUTION'
          ELSE 'HIGH_RISK_PARAMETER'
        END AS risk_type,
        datetime(start_time) AS captured_at
      FROM flows
      WHERE
        lower(req_url) LIKE '%redirect=%' OR lower(req_url) LIKE '%url=%' OR lower(req_url) LIKE '%next=%'
        OR lower(req_url) LIKE '%admin=%' OR lower(req_url) LIKE '%role=%' OR lower(req_url) LIKE '%privilege=%'
        OR lower(req_url) LIKE '%token=%' OR lower(req_url) LIKE '%secret=%' OR lower(req_url) LIKE '%key=%'
        OR lower(req_url) LIKE '%file=%' OR lower(req_url) LIKE '%path=%' OR lower(req_url) LIKE '%cmd=%'
      ORDER BY start_time DESC
      LIMIT 100;
    "
    ;;

  triage-errors)
    sqlite3 -json "$DB" "
      SELECT
        id,
        host,
        req_method,
        req_url,
        resp_status,
        duration_ns / 1000000 AS duration_ms,
        scope_status,
        CAST(substr(resp_body, 1, 500) AS TEXT) AS response_snippet,
        datetime(start_time) AS captured_at
      FROM flows
      WHERE resp_status >= 500
      ORDER BY start_time DESC
      LIMIT 50;
    "
    ;;

  triage-auth)
    sqlite3 -json "$DB" "
      SELECT
        id,
        host,
        req_method,
        req_url,
        resp_status,
        scope_status,
        CASE
          WHEN resp_status = 401 THEN 'UNAUTHORIZED'
          WHEN resp_status = 403 THEN 'FORBIDDEN'
          WHEN req_headers LIKE '%Authorization%' OR req_headers LIKE '%Bearer%' THEN 'AUTHENTICATED_REQUEST'
          ELSE 'AUTH_CHECK'
        END AS auth_state,
        datetime(start_time) AS captured_at
      FROM flows
      WHERE resp_status IN (401, 403) OR req_headers LIKE '%Bearer%' OR req_headers LIKE '%Authorization%'
      ORDER BY start_time DESC
      LIMIT 50;
    "
    ;;

  suspicious)
    sqlite3 -json "$DB" "
      SELECT
        a.flow_id,
        f.host,
        f.req_method,
        f.req_url,
        f.resp_status,
        a.summary,
        a.provider,
        a.model,
        datetime(a.created_at) as analyzed_at
      FROM analyses a
      JOIN flows f ON f.id = a.flow_id
      WHERE
        lower(a.summary) LIKE '%critical%'
        OR lower(a.summary) LIKE '%high%severity%'
        OR lower(a.summary) LIKE '%suspicious%'
        OR lower(a.summary) LIKE '%sql injection%'
        OR lower(a.summary) LIKE '%xss%'
        OR lower(a.summary) LIKE '%ssrf%'
        OR lower(a.summary) LIKE '%path traversal%'
        OR lower(a.summary) LIKE '%directory traversal%'
        OR lower(a.summary) LIKE '%remote code%'
        OR lower(a.summary) LIKE '%command injection%'
        OR lower(a.summary) LIKE '%privilege escalat%'
        OR lower(a.summary) LIKE '%authentication bypass%'
      ORDER BY a.created_at DESC;
    "
    ;;

  # ══════════════════════════════════════════════════════════════════════════════
  # LAYER 4: DEEP PAYLOAD & FLOW INSPECTION
  # ══════════════════════════════════════════════════════════════════════════════

  flow|inspect)
    id="${1:-}"
    if [[ -z "$id" ]]; then
      echo >&2 "Usage: query.sh flow <flow_id_or_partial_id>"
      exit 1
    fi
    sqlite3 -json "$DB" "
      SELECT
        id,
        host,
        scheme,
        req_method,
        req_url,
        req_version,
        req_headers,
        CAST(req_body AS TEXT) AS req_body,
        resp_status,
        resp_version,
        resp_headers,
        CAST(resp_body AS TEXT) AS resp_body,
        scope_status,
        state,
        error,
        datetime(start_time) AS captured_at,
        duration_ns / 1000000 AS duration_ms
      FROM flows
      WHERE id LIKE '%${id}%'
      LIMIT 1;
    "
    ;;

  body)
    id="${1:-}"
    part="${2:-resp}" # req or resp
    if [[ -z "$id" ]]; then
      echo >&2 "Usage: query.sh body <flow_id> [req|resp]"
      exit 1
    fi
    col="resp_body"
    [[ "$part" == "req" ]] && col="req_body"
    sqlite3 "$DB" "SELECT CAST($col AS TEXT) FROM flows WHERE id LIKE '%${id}%' LIMIT 1;"
    ;;

  headers)
    id="${1:-}"
    part="${2:-resp}" # req or resp
    if [[ -z "$id" ]]; then
      echo >&2 "Usage: query.sh headers <flow_id> [req|resp]"
      exit 1
    fi
    col="resp_headers"
    [[ "$part" == "req" ]] && col="req_headers"
    sqlite3 -json "$DB" "SELECT $col FROM flows WHERE id LIKE '%${id}%' LIMIT 1;"
    ;;

  curl)
    id="${1:-}"
    if [[ -z "$id" ]]; then
      echo >&2 "Usage: query.sh curl <flow_id>"
      exit 1
    fi
    flow_json="$(sqlite3 -json "$DB" "
      SELECT req_method, req_url, scheme, host, req_headers, CAST(req_body AS TEXT) as req_body
      FROM flows WHERE id LIKE '%${id}%' LIMIT 1;
    ")"

    if [[ "$flow_json" == "[]" || -z "$flow_json" ]]; then
      echo >&2 "Flow not found: $id"
      exit 1
    fi

    # Output curl command safely via stdin
    echo "$flow_json" | python3 -c "
import json, sys, shlex

try:
    raw = sys.stdin.read()
    data = json.loads(raw)[0]
except Exception as e:
    print(f'Error parsing flow JSON: {e}', file=sys.stderr)
    sys.exit(1)

method = data.get('req_method') or 'GET'
url = data.get('req_url') or ''
scheme = data.get('scheme') or 'https'
host = data.get('host') or ''
body = data.get('req_body') or ''

if not url.startswith('http'):
    url = f'{scheme}://{host}{url}'

cmd = ['curl', '-i', '-X', method, url]

headers_raw = data.get('req_headers')
if headers_raw:
    try:
        headers = json.loads(headers_raw) if isinstance(headers_raw, str) else headers_raw
        for k, v in headers.items():
            if k.lower() in ['host', 'content-length']: continue
            val = v[0] if isinstance(v, list) and v else str(v)
            cmd.extend(['-H', f'{k}: {val}'])
    except Exception:
        pass

if body:
    cmd.extend(['--data-raw', body])

print(' '.join(shlex.quote(c) for c in cmd))
"
    ;;

  diff)
    id1="${1:-}"
    id2="${2:-}"
    if [[ -z "$id1" || -z "$id2" ]]; then
      echo >&2 "Usage: query.sh diff <flow_id_1> <flow_id_2>"
      exit 1
    fi
    sqlite3 -json "$DB" "
      SELECT id, req_method, host, req_url, resp_status, scope_status,
             req_headers, CAST(req_body AS TEXT) as req_body,
             resp_headers, CAST(resp_body AS TEXT) as resp_body
      FROM flows WHERE id LIKE '%${id1}%' OR id LIKE '%${id2}%'
      ORDER BY start_time;
    "
    ;;

  # ══════════════════════════════════════════════════════════════════════════════
  # LAYER 5: AI ANALYSES & ADVISORY
  # ══════════════════════════════════════════════════════════════════════════════

  analyses)
    sqlite3 -json "$DB" "
      SELECT
        a.id,
        a.flow_id,
        f.host,
        f.req_method,
        f.req_url,
        a.kind,
        a.provider,
        a.model,
        a.summary,
        datetime(a.created_at) as created_at
      FROM analyses a
      JOIN flows f ON f.id = a.flow_id
      ORDER BY a.created_at DESC
      LIMIT 50;
    "
    ;;

  analysis)
    flow_id="${1:-}"
    if [[ -z "$flow_id" ]]; then
      echo >&2 "Usage: query.sh analysis <flow_id>"
      exit 1
    fi
    sqlite3 -json "$DB" "
      SELECT
        a.id,
        a.flow_id,
        a.kind,
        a.provider,
        a.model,
        a.summary,
        a.raw_json,
        datetime(a.created_at) as created_at
      FROM analyses a
      WHERE a.flow_id LIKE '%${flow_id}%'
      ORDER BY a.created_at DESC;
    "
    ;;

  scope)
    sqlite3 -json "$DB" "
      SELECT
        r.id,
        COALESCE(p.name, 'Global') as preset,
        r.kind,
        r.pattern,
        r.match_mode,
        r.action,
        r.enabled,
        r.priority,
        r.note
      FROM scope_rules r
      LEFT JOIN scope_presets p ON p.id = r.preset_id
      ORDER BY r.priority DESC, r.created_at;
    " 2>/dev/null || sqlite3 -json "$DB" "
      SELECT id, kind, pattern, match_mode, action, enabled, priority, note
      FROM scope_rules ORDER BY priority DESC;
    "
    ;;

  *)
    echo >&2 "Unknown command: $cmd"
    echo >&2 ""
    echo >&2 "Ouroboros Layered Investigation Commands:"
    echo >&2 ""
    echo >&2 "  Layer 1 - Overview & Index:"
    echo >&2 "    overview                 - Project traffic statistics & breakdown"
    echo >&2 "    presets                  - List all named scope presets"
    echo >&2 "    hosts                    - List captured hosts with flow counts"
    echo >&2 "    flows [--limit N]        - List captured flows"
    echo >&2 ""
    echo >&2 "  Layer 2 - Filtered Discovery:"
    echo >&2 "    filter [--host H] [--path P] [--status S] [--search Q] - Search flows"
    echo >&2 "    endpoints [--host H]     - Discovered API endpoints & methods"
    echo >&2 "    params [--host H]        - Discovered query parameters"
    echo >&2 ""
    echo >&2 "  Layer 3 - Progressive Security Triaging:"
    echo >&2 "    triage                   - Full security triage summary across all categories"
    echo >&2 "    triage-injections        - Attack signatures (SQLi, XSS, SSRF, Path Traversal)"
    echo >&2 "    triage-params            - Sensitive parameter names (redirect, token, admin, etc.)"
    echo >&2 "    triage-errors            - 5xx internal server errors"
    echo >&2 "    triage-auth              - 401/403 auth errors and Bearer token requests"
    echo >&2 "    suspicious               - LLM-flagged suspicious flows"
    echo >&2 ""
    echo >&2 "  Layer 4 - Deep Payload Inspection:"
    echo >&2 "    flow <id>                - Full request & response inspection"
    echo >&2 "    body <id> [req|resp]     - View raw request/response body"
    echo >&2 "    headers <id> [req|resp]  - View request/response headers"
    echo >&2 "    curl <id>                - Generate ready-to-run curl command"
    echo >&2 "    diff <id1> <id2>         - Compare two flows"
    echo >&2 ""
    echo >&2 "  Layer 5 - AI & Scope:"
    echo >&2 "    analyses                 - List AI security analysis results"
    echo >&2 "    analysis <flow_id>       - Get AI analysis for specific flow"
    echo >&2 "    scope                    - View active scope rules"
    exit 1
    ;;
esac
