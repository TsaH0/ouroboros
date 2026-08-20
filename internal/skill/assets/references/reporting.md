# Reporting Findings

A finding that isn't clearly reported often gets triaged as N/A even when it's real. After confirming a finding manually, write it up with:

1. **Title** — one line, specific: "Reflected XSS on `search` parameter at `/search`" not "XSS vulnerability".
2. **Severity / CVSS estimate** — use the program's stated severity model if given (many use CVSS 3.1 or 4.0); otherwise state your own reasoning briefly.
3. **Affected asset(s)** — exact URL(s)/endpoint(s), and whether other subdomains share the same vulnerable code path.
4. **Steps to reproduce** — numbered, minimal, exact (request method, full URL, parameters, headers if relevant, cookies/auth state needed). Someone with zero context should be able to follow it and get the same result.
5. **Proof of Concept** — a raw HTTP request (curl or Burp `-r` style), plus a screenshot or short screen recording where visual proof matters (XSS alert box, CORS response headers, redirect chain).
6. **Impact** — what a real attacker gains: account takeover, data exfiltration, session hijack, etc. Be concrete and avoid inflating impact beyond what's demonstrated.
7. **Suggested remediation** — brief, standard fix (e.g. "encode output in the search results template", "restrict `Access-Control-Allow-Origin` to an explicit allow-list instead of reflecting `Origin`").

## Tips that improve triage speed / bounty outcome

- One vulnerability per report. Don't bundle unrelated findings — it slows triage and can undervalue the bounty.
- Attach the raw request/response (from Burp, or `curl -v`) rather than just a description — triagers reproduce fastest from raw HTTP.
- If the same bug class affects many subdomains (e.g. reflected XSS on a shared template used across 40 subdomains), report the root cause once and list all affected assets rather than filing 40 separate reports — check the program's duplicate policy first.
- Redact/avoid touching real user data in screenshots and PoCs — use your own test account wherever possible.
- Re-read the program's scope and out-of-scope/"not accepted" list before submitting — a lot of automated-scanner findings (missing security headers, verbose error messages, self-XSS) are explicitly excluded by most programs.
