---
name: ouroboros-advisor
description: >
  Query the Ouroboros intercepting proxy database to inspect captured HTTP
  traffic, perform progressive multi-layered security triaging, identify suspicious
  requests, check scope status, and retrieve AI analysis results. Use this skill
  when the user asks about what requests have been captured, wants to know which
  requests look suspicious, wants an automated security audit of intercepted traffic,
  or needs advice on vulnerabilities.
---

# Ouroboros Advisor Skill

## Overview
Ouroboros records all captured HTTP flows, scope presets/rules, and AI analyses in a local SQLite database at `~/.config/ouroboros/ouroboros.db`.

This skill equips agents with a **Multi-Layer Progressive Security Triaging Tool** (`query.sh`) to audit traffic step-by-step:
1. **Layer 1**: Project & Traffic Overview (hosts, counts, status breakdown, flow index)
2. **Layer 2**: Filtered Discovery (slice traffic by host, status, method, path, search query, endpoints, params)
3. **Layer 3**: Progressive Security Triage (detect injection signatures, sensitive parameter patterns, server errors, auth anomalies)
4. **Layer 4**: Deep Payload Inspection (full request/response details, raw bodies, curl generation, diffs)
5. **Layer 5**: AI Analyses & Remediation Advice
6. **Layer 6**: Bug Bounty Methodology & Thinking Framework
7. **Layer 7**: 404 Tech Stack Fingerprinting
8. **Layer 8**: SearchSploit Structured LLM Commands
9. **Layer 9**: Triage Validation 7-Question Gate
10. **Layer 10**: Active Recon Pipeline (subfinder/httpx/nuclei) — companion to passive triage

## How To Use

Run the helper script:
```bash
bash ~/.gemini/antigravity-cli/skills/ouroboros-advisor/scripts/query.sh <command> [args]
bash /home/Tejesh/Projects/ouroboros/skills/ouroboros-advisor/scripts/query.sh <command> [args]
bash ~/.agents/skills/ouroboros-advisor/scripts/query.sh <command> [args]
```

---

## Agent Step-by-Step Investigation Workflow

### Layer 1: Survey the Project & Traffic Landscape
```bash
bash .../query.sh overview
bash .../query.sh hosts
bash .../query.sh flows --limit 30
bash .../query.sh scope
```

### Layer 2: Filter Down to Target Subsets
```bash
bash .../query.sh endpoints --host api.example.com
bash .../query.sh params --host example.com
bash .../query.sh filter --method POST --host api.example.com
bash .../query.sh filter --status 500
bash .../query.sh filter --search "admin"
bash .../query.sh filter --host hackerone.com --search "graphql"
```

### Layer 3: Progressive Security & Anomaly Triage
```bash
bash .../query.sh triage
bash .../query.sh triage-injections
bash .../query.sh triage-params
bash .../query.sh triage-errors
bash .../query.sh triage-auth
bash .../query.sh suspicious
```

### Layer 4: Deep Payload & Request Inspection
```bash
bash .../query.sh flow <flow_id>
bash .../query.sh body <flow_id> resp
bash .../query.sh body <flow_id> req
bash .../query.sh headers <flow_id>
bash .../query.sh curl <flow_id>
bash .../query.sh diff <flow_id_1> <flow_id_2>
```

### Layer 5: Retrieve AI Analyses & Scope Rules
```bash
bash .../query.sh analyses
bash .../query.sh analysis <flow_id>
bash .../query.sh scope
```

---

## Layer 6: Bug Bounty Hunting Methodology (adapted from claude-bug-bounty)

### Mindset — Define, Select, Execute
Before querying, answer:
1. **Define**: "Today I target [feature/host] to achieve [CIA/ATO/RCE]" — e.g., "target hackerone.com/graphql to achieve IDOR / confidentiality"
2. **Select**: Choose 1-2 vuln classes (IDOR, SSRF, XSS, SQLi, etc.) — focus ONLY on those
3. **Execute**: Translate every hypothesis into a single reproducible `curl` from `query.sh curl <id>` with a modified payload

### 5 Ultimate Goals (pick one per session)
Confidentiality (steal data), Integrity (modify), Availability (DoS), Account Takeover, RCE

### 4 Thinking Domains for Intercepted Traffic
- **Critical**: Frontend disabled? Send via repeater directly. `user_role=user` cookie → try `admin`. `price=1000` → try `1`. Is there a trust boundary the UI enforces but the API doesn't?
- **Multi-perspective**: Horizontal (user A token + user B id → IDOR), Vertical (user → /admin/deleteUser), Data flow (hidden JSON `debug=false`, `discount_rate`), Time/State (race on coupon), Client env (mobile UA → legacy API)
- **Tactical — Anomaly detection in flows**: Naming anomaly (`userId` vs `user_id` → different dev), Error diff (same 403 but different JSON → different backend), `200` but tiny body/"just a moment" → WAF soft block (verify via repeater), Version diff (JS before/after → new endpoints), Supply chain (framework version in headers → CVE)
- **Strategic**: Defender must patch all; you need one. Log "feels wrong" and verify later. Use model to expand hypotheses, not to declare verdicts — every AI suggestion must become a `curl` experiment.

### 5-Phase Non-Linear Workflow (mapped to Ouroboros)
```
Recon (query.sh overview/hosts) → Map (endpoints/params) → Find (triage-injections/params) → Prove (curl + repeater edit + diff) → Report (query.sh analyses)
Non-linear: stuck at Prove → back to Map (new endpoint found), WAF blocks → need new payload.
```
- **20-min rotation**: Not progressing on one param/endpoint? Rotate to next.
- **After every triage**: `query.sh endpoints` + `query.sh params` to route leads; never lose a lead.

### Tool Routing by Phase (Ouroboros-native)
| Phase | Ouroboros commands |
|-------|-------------------|
| Recon | `overview`, `hosts`, `flows --limit 50` |
| Map | `endpoints --host X`, `params --host X`, `flow <id>` headers/body |
| Find | `triage`, `triage-injections`, `triage-params`, `triage-auth`, `filter --search` |
| Prove | `curl <id>` → edit in repeater (`r` in TUI) or manual curl, `diff <id1> <id2>` |
| Escalate | Chain: XSS→cookie steal→ATO, IDOR→PII scrape, SSRF→169.254.169.254 metadata |

---

## Layer 7: 404 Tech Stack Fingerprinting (from 0xdf 404 cheatsheet)

When `query.sh flow <id>` shows a 404 body, fingerprint the stack before choosing payloads. Request a non-existent path via repeater (`GET /0xdf-404-test-12345`) and compare body to these signatures:

| Stack | Default 404 body signature |
|-------|---------------------------|
| nginx | `<html><head><title>404 Not Found</title></head><body><center><h1>404 Not Found</h1></center><hr><center>nginx/1.24.0` |
| Apache | `Apache/2.4.41 (Ubuntu) Server at` + `<address>` |
| IIS | `404 - File or directory not found.` + `#header{background-color:#555555}` |
| Flask/Werkzeug | `The requested URL was not found on the server. If you entered the URL manually please check your spelling` |
| Django | `<h1>Not Found</h1><p>The requested resource was not found on this server.</p>` |
| FastAPI | `{"detail":"Not Found"}` (JSON, Firefox formats it) |
| AIOHTTP | `404: Not Found` plaintext |
| Fiber | `Cannot GET /path` | Gin | `404 page not found` | PHP-FPM | `File not found.` |
| Laravel | Tailwind CSS `normalize.css` + `404 Not Found` side-by-side | Symfony | `Oops! An Error Occurred` + `The server returned a "404 Not Found".` |
| Express | `<pre>Cannot GET /path</pre>` | NextJS | `404: This page could not be found` + `__NEXT_DATA__` with `/_error` |
| Tomcat | `HTTP Status 404 – Not Found` + `Apache Tomcat/9.0.31` + `The origin server did not find a current representation` |
| Spring Boot | `Whitelabel Error Page` + `There was an unexpected error (type=Not Found, status=404)` |
| Jetty | `Error 404 - Not Found.` + `No context on this server matched` OR `Default404Servlet-` table |
| Ruby on Rails | via `X-Runtime: Ruby` header | PHP stack | `X-Powered-By: PHP/7.4` |

**Stack → Hunt priority:**
- Rails → Mass assignment, IDOR `:id`; Django → IDOR, SSTI `mark_safe`; Flask → SSTI `render_template_string`, SSRF; Laravel → `$fillable` mass assignment; Express → prototype pollution; Spring Boot → `/actuator/*`; Next.js → `/_next/data/`; GraphQL → introspection `__schema`.

**Agent action:** After identifying stack via 404, run `filter --host <target> --search "404"` and `body <id> resp` to confirm, then select payloads matching stack (e.g., Jinja2 `{{7*7}}` for Flask vs `{{_self.env...}}` for Twig).

---

## Layer 8: SearchSploit Structured LLM Commands

Ouroboros recon already maps tech via headers; when a stack version is identified (e.g., `Apache/2.4.41`, `nginx/1.24.0`, `Spring Boot`), use searchsploit locally with JSON for structured parsing.

### A. Querying (JSON — primary for LLM)
```bash
# Primary: structured JSON (EDB-ID, Title, Path, Type, Platform)
searchsploit "Apache 2.4.41" --json
searchsploit "Spring Boot" --json | jq '.RESULTS_EXPLOIT[] | {Title, EDB_ID, Path}'

# CVE direct
searchsploit --cve 2021-44228 --json
searchsploit --cve CVE-2023-44487 --json

# Exact title (disable fuzzy) — reduce false positives when version known
searchsploit -e "WordPress 6.4.2" --json
searchsploit -e "Apache Tomcat 9.0.31" --json

# Exclude noise (PoC/dos)
searchsploit "Tomcat 9" --exclude="(PoC)|/dos/" --json
```

### B. Inspection & File Retrieval
```bash
# Locate absolute path
searchsploit -p 12345
searchsploit -p 50406

# Copy to workspace for reading/editing
searchsploit -m 12345          # copies to ./

# Print directly to stdout for LLM analysis
searchsploit -x 12345
searchsploit -x 50406 | head -100

# Typical flow: JSON → pick EDB-ID → -x to read → check if stack matches captured flows
```

### C. Database Update
```bash
searchsploit -u   # pull latest from GitHub — run weekly
```

### Ouroboros integration pattern
```bash
# 1. Fingerprint via 404 or header
bash .../query.sh flow <id> | grep -i "404 Not Found\|nginx\|Tomcat"
bash .../query.sh headers <id> | grep -i "Server|X-Powered-By"

# 2. Search
searchsploit "nginx 1.24.0" --json | jq .

# 3. Retrieve and compare to captured traffic
searchsploit -x <EDB-ID> | grep -i "payload\|curl"
bash .../query.sh curl <flow_id>   # adapt payload to actual endpoint
```

---

## Layer 9: Triage Validation — 7-Question Gate (kill fast)

Before claiming a finding is vuln, run this gate. One fail = kill.

**Q1: Can attacker use it RIGHT NOW?** Write: Setup (account needed), Request (exact curl from `query.sh curl`), Result (what data shows), Impact, Cost. If can't write Request → kill.

**Q2: Is impact on program's accepted list?** Check program scope. Critical: ATO without interaction, RCE, SQLi with exfil. If in "Out of Scope" → kill.

**Q3: Is root cause in in-scope asset?** Verify `host` from `query.sh hosts` is in `query.sh scope` preset. Out-of-scope → kill.

**Q4: Does it require privileged access attacker can't get?** "Admin can do X" = kill (unless priv-esc). "Non-admin can do admin X" = valid.

**Q5: Is it already known/accepted?** Search program's disclosed reports + `filter --search` for same endpoint+class. If duplicate → kill.

**Q6: Can you prove impact beyond "technically possible"?** XSS → show cookie theft, not just `alert(1)`. SSRF → show internal data, not just DNS ping. IDOR → show other user's PII, not just 200. If only technically possible → downgrade.

**Q7: Is it a known-invalid class without chain?** Never submit alone: missing CSP/HSTS, GraphQL introspection alone, Clickjacking non-sensitive, Self-XSS, Open redirect alone, SSRF DNS only, Host header alone, Rate limit on search. Need chain: open redirect+OAuth→ATO, SSRF DNS+metadata→Critical.

**Pre-submit gates:** Reality Check (reproducible), Impact Validation, Deduplication, Report Quality (title: `[Bug Class] in [Endpoint] allows [actor] to [impact]`, copy-paste curl, <600 words, CVSS matching impact).

**CVSS quick:** IDOR read PII 6.5 Medium, IDOR write 7.5 High, Auth bypass admin 9.8 Critical, Stored XSS 8.8 High, SQLi dump 8.6 High, SSRF metadata 9.1 Critical.

---

## Layer 10: Active Recon Pipeline (companion to passive triage)

Ouroboros captures **passive** traffic (what the browser sends). For **active** recon — subdomain enum, live-host probing, tech fingerprinting, URL/JS discovery, parameter discovery, and vuln scanning — use the standard CLI tool pipeline. Each phase has a reference file in `references/` with exact commands, flags, and gotchas.

### Scope & authorization gate (every time)
1. Target must be in an authorized program (HackerOne/Bugcrowd/Intigriti listing or private engagement letter). Only test in-scope assets.
2. Check rate limits / testing restrictions. Start at 10-20 threads, not 200. Many programs forbid automated scanners.
3. No destructive testing (`sqlmap --os-shell`, mass writes, DoS fuzzing) unless explicitly allowed.

### Pipeline phases

| Phase | Goal | Primary tools | Reference |
|---|---|---|---|
| 1. Subdomain enum | Find all subdomains | `subfinder`, `amass`, `assetfinder`, `crt.sh` | `references/subdomain-enum.md` |
| 2. Live host / port probing | Which hosts respond, ports, tech | `httpx`, `naabu`, `dnsx` | `references/http-probing-and-fingerprinting.md` |
| 3. Tech fingerprinting | Stack, CMS, WAF | `whatweb`, `httpx -tech-detect`, `wappalyzer-cli` | `references/http-probing-and-fingerprinting.md` |
| 4. URL / endpoint / JS discovery | Historical + crawled URLs, JS endpoints | `waybackurls`, `gau`, `katana`, `hakrawler`, `getJS`, `LinkFinder` | `references/url-and-endpoint-discovery.md` |
| 5. Parameter & content discovery | Hidden params, dirs, files | `arjun`, `paramspider`, `ffuf`, `gobuster`, `feroxbuster` | `references/parameter-and-content-discovery.md` |
| 6. Vulnerability scanning | XSS, CORS, SQLi, SSRF, open redirect, CRLF, takeovers, CVEs | `nuclei`, `dalfox`, `XSStrike`, `Corsy`, `sqlmap`, `subzy`, `crlfuzz` | `references/vuln-scanning.md` |
| 7. Visual recon / triage | Screenshot everything | `gowitness`, `aquatone` | `references/vuln-scanning.md` |
| 8. Reporting | Write up for fast triage | — | `references/reporting.md` |

### Ouroboros integration

Ouroboros has a built-in recon engine (Recon tab `5`) that runs `subfinder`, `gau`, `waybackurls`, `whatweb`, `searchsploit` directly. Use it for phases 1, 4, 3, and CVE lookup. For phases 2, 5, 6, 7 (active probing, fuzzing, vuln scanning, screenshots) use the external CLI tools above.

**Workflow:**
1. Passive: `query.sh overview` + `query.sh hosts` + `query.sh endpoints` to map what the browser already touched.
2. Active: run the recon pipeline (phases 1-7) to expand the attack surface beyond what was browsed.
3. Feed active results back: browse discovered endpoints through the Ouroboros proxy → captured in DB → `query.sh triage` finds anomalies.
4. Prove: `query.sh curl <id>` → edit in repeater (`r` in TUI) → `diff <id1> <id2>`.

### Quick end-to-end (tools installed, scope confirmed)

```bash
export TARGET=example.com
mkdir -p recon/$TARGET/{subdomains,httpx,urls,js,params,content,screenshots,vulns,reports}
cd recon/$TARGET

subfinder -d $TARGET -silent | tee subdomains/all.txt \
  | httpx -silent -tech-detect -status-code -title -o httpx/live.txt

cat httpx/live.txt | cut -d' ' -f1 > httpx/live_urls.txt
waybackurls $TARGET | tee urls/wayback.txt
gau $TARGET | tee urls/gau.txt
cat urls/*.txt | sort -u > urls/all.txt
nuclei -l httpx/live_urls.txt -t cves/ -o vulns/nuclei.txt
dalfox file urls/all.txt -o vulns/dalfox.txt
```

### Check installed tools
```bash
for t in subfinder amass assetfinder httpx naabu waybackurls gau katana whatweb ffuf gobuster nuclei dalfox sqlmap gowitness; do
  command -v $t >/dev/null 2>&1 && echo "$t: installed" || echo "$t: MISSING"
done
```

### Go tools offline note
Most recon tools are Go binaries. If `proxy.golang.org` is unreachable:
```bash
export GOPROXY=direct
go install -v github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest
```

Read the relevant `references/*.md` for exact commands per phase. Don't hold all phases in context at once — pull in references as you move through the pipeline.

