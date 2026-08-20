# Vulnerability Scanning

Run these against the **live URL / parameterized URL lists** produced by the earlier phases, only against confirmed in-scope targets, and always start with the lowest-impact/read-only scanners (`nuclei`) before anything that sends exploit-style payloads.

## nuclei (template-based scanner — run this first, broadest coverage)

```bash
export GOPROXY=direct
go install -v github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest
nuclei -update-templates

# broad low-noise pass: known CVEs, exposed panels/files, misconfig
nuclei -l httpx/live_urls.txt -tags cve,exposure,misconfig,default-login \
  -severity medium,high,critical -o vulns/nuclei.txt

# tech-specific pass, once you know the stack from fingerprinting
nuclei -l httpx/live_urls.txt -tags wordpress -o vulns/nuclei_wp.txt
```
Thousands of community-maintained templates; the highest signal-to-noise scanner in this list and the right default first pass before anything more targeted.

## dalfox (XSS scanner — fast, actively maintained, good default XSS tool)

```bash
go install -v github.com/hahwul/dalfox/v2@latest

# single URL
dalfox url "https://app.example.com/search?q=test"

# bulk against every parameterized URL found in recon
dalfox file urls/params_urls.txt -o vulns/dalfox.txt --silence
```
Handles reflected/DOM XSS detection with context-aware payload generation and can verify via headless browser (`--waf-evasion` / `-b` for blind XSS via an external collaborator like `interact.sh`).

## XSStrike (older, still useful for manual-style deep single-target XSS testing)

```bash
git clone https://github.com/s0md3v/XSStrike.git && cd XSStrike
pip install -r requirements.txt --break-system-packages

python3 xsstrike.py -u "https://app.example.com/search?q=test" --crawl
```
Good for its fuzzing/context-analysis engine and WAF detection; slower than dalfox, best used on a shortlist of promising endpoints rather than bulk scanning.

## Corsy (CORS misconfiguration scanner)

```bash
git clone https://github.com/s0md3v/Corsy.git && cd Corsy
pip install -r requirements.txt --break-system-packages

python3 corsy.py -i ../httpx/live_urls.txt -o ../vulns/corsy.txt
```
Flags reflected-origin, null-origin-allowed, and wildcard-with-credentials misconfigs — all common, high-signal bug bounty findings.

## sqlmap (SQL injection — use narrowly and carefully)

```bash
sudo apt install -y sqlmap   # or: pip install sqlmap --break-system-packages

# always start with the safest detection-only mode, never automate --batch at scale
sqlmap -u "https://app.example.com/item?id=1" --batch --level=1 --risk=1 --dbs
```
- Only run against endpoints you've manually confirmed look injectable (error messages, odd response-time behavior) — mass-running sqlmap across every discovered URL is noisy, slow, and against most programs' rules.
- Keep `--risk` and `--level` low initially; higher levels send payloads that can modify data (`--risk=3` includes `OR`-based and time-based heavy payloads) — never use `--risk=3` or destructive flags (`--os-shell`, `--file-write`) without explicit program permission.
- Prefer `-r request.txt` (raw request from Burp/browser dev tools) over guessing params, for accuracy and lower request volume.

## subzy (subdomain takeover checker)

```bash
go install -v github.com/LukaSikic/subzy@latest
subzy run --targets subdomains/resolved.txt --output vulns/subzy.txt
```
Checks for dangling CNAMEs pointing at unclaimed services (S3, GitHub Pages, Heroku, etc.) — reliable, low-noise, high-severity-when-found bug class.

## crlfuzz (CRLF injection scanner)

```bash
go install -v github.com/dwisiswant0/crlfuzz/cmd/crlfuzz@latest
cat httpx/live_urls.txt | crlfuzz -o vulns/crlf.txt
```

## gowitness / aquatone (screenshot everything for fast manual triage)

```bash
go install -v github.com/sensepost/gowitness@latest
gowitness scan file -f httpx/live_urls.txt --screenshot-path screenshots/

# or aquatone (older, still widely used)
cat httpx/live_urls.txt | aquatone -out screenshots/
```
Not a scanner, but essential — after a big recon run, a photo grid of every live host is the fastest way to eyeball admin panels, login forms, default pages, and error pages worth a manual look.

## Notes
- **Confirm, don't just report scanner output.** `nuclei`/`dalfox`/`sqlmap` all produce false positives. Manually verify a finding (reproduce it, capture a clean PoC) before it goes in a report — see `reporting.md`.
- If a WAF was detected in the fingerprinting phase, expect scanner payloads to get blocked/rate-limited; slow down (`-rate-limit`, `-c` concurrency flags) rather than trying to bypass the WAF outright unless WAF-bypass is explicitly in scope for the engagement.
- Chain the highest-value combo when time is short: `nuclei` (broad) → `dalfox` on param URLs (XSS) → `Corsy` on live hosts (CORS) → `subzy` on all subdomains (takeover). These four cover a large share of common bug bounty submissions with low false-positive rates.
